package iscsi

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Connection is a data structure holding all of the info related to
// an iSCSI connection
type Connection struct {
	Device      string
	IQN         string
	MPDevice    string
	Host        string
	Channel     string
	FileSystem  string
	ChapEnabled bool
	Portal      string
	Port        string
	TgtIQN      string
	Lun         string
	ChapLogin   string
	ChapPasswd  string
}

// Device is a dummy
type Device struct {
	IQN        string
	Path       string
	MPDevice   string
	Portal     string
	Port       string
	IFace      string
	UseChap    bool
	ChapLogin  string
	ChapPasswd string
	Lun        int
}

var (
	Trace   *log.Logger
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func init() {
	Trace = log.New(os.Stdout,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(os.Stdout,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(os.Stdout,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(os.Stdout,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

func isMultipath(dev string) bool {
	args := []string{"-c", dev}
	out, err := exec.Command("multipath", args...).CombinedOutput()
	if err != nil {
		Error.Println("multipath check failed, multipath not running?")
		return false
	}
	Trace.Printf("response from multipath cmd: %s", out)
	if strings.Contains(string(out), "is a valid multipath device") {
		Trace.Printf("returning isMultipath == true\n")
		return true
	}
	return false
}

func stat(f string) (string, error) {
	out, err := exec.Command("sh", "-c", (fmt.Sprintf("stat %s", f))).CombinedOutput()
	return string(out), err

}

func waitForPathToExist(d string, maxRetries int) bool {
	for i := 0; i < maxRetries; i++ {
		if _, err := stat(d); err == nil {
			return true
		}
		time.Sleep(time.Second * time.Duration(2*1))
	}
	return false
}

// Attach performs an attachment of the specified resource
func Attach(d *Device) (Device, error) {
	// Verify iscsi initiator tools are available
	out, err := exec.Command("iscsiadm", []string{"-m", "iface", "-I", d.IFace, "-o", "show"}...).CombinedOutput()
	if err != nil {
		Error.Printf("iscsi unable to read from interface %s, error: %s", d.IFace, string(out))
		return Device{}, err
	}

	// Make sure we're not already attached
	path := "/dev/disk/by-path/ip-" + d.Portal + "-iscsi-" + d.IQN + "lun-" + strconv.Itoa(d.Lun)
	if waitForPathToExist(path, 0) == true {
		d.Path = path
		return *d, nil
	}

	if d.UseChap == true {
		err := LoginWithChap(d.IQN, d.Portal, d.ChapLogin, d.ChapPasswd, d.IFace)
		if err != nil {
			Error.Printf("error: %+v", err)
		}
	} else {
		err := Login(d.IQN, d.Portal, d.IFace)
		if err != nil {
			Error.Printf("error: %+v", err)
		}
	}

	if waitForPathToExist(path, 10) {
		d.Path = path
	}

	return *d, nil
}

// GetInitiatorIqns queries the system to obtain the list of configured initiator names
// and returns them to the caller
func GetInitiatorIqns() ([]string, error) {
	var iqns []string
	out, err := exec.Command("cat", "/etc/iscsi/initiatorname.iscsi").CombinedOutput()
	if err != nil {
		Error.Printf("unable to gather initiator names: %v\n", err)
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if strings.Contains(l, "InitiatorName=") {
			iqns = append(iqns, strings.Split(l, "=")[1])
		}
	}
	return iqns, nil
}

// LoginWithChap performs the necessary iscsiadm commands to log in to the
// specified iSCSI target.  This wrapper will create a new node record, setup
// CHAP credentials and issue the login command
func LoginWithChap(tiqn, portal, username, password, iface string) error {
	args := []string{"-m", "node", "-T", tiqn, "-p", portal}
	createArgs := append(args, []string{"--interface", iface, "--op", "new"}...)

	if _, err := exec.Command("iscsiadm", createArgs...).CombinedOutput(); err != nil {
		return err
	}

	authMethodArgs := append(args, []string{"--op=update", "--name", "node.session.auth.authmethod", "--value=CHAP"}...)
	if out, err := exec.Command("iscsiadm", authMethodArgs...).CombinedOutput(); err != nil {
		Error.Printf("output of failed iscsiadm cmd: %+v\n", out)
		return err
	}

	authUserArgs := append(args, []string{"--op=update", "--name", "node.session.auth.username", "--value=" + username}...)
	if _, err := exec.Command("iscsiadm", authUserArgs...).CombinedOutput(); err != nil {
		return err
	}
	authPasswordArgs := append(args, []string{"--op=update", "--name", "node.session.auth.password", "--value=" + password}...)
	if _, err := exec.Command("iscsiadm", authPasswordArgs...).CombinedOutput(); err != nil {
		return err
	}

	// Finally do the login
	loginArgs := append(args, []string{"--login"}...)
	_, err := exec.Command("iscsiadm", loginArgs...).CombinedOutput()
	return err
}

// Login performs a simple iSCSI login (devices that do not use CHAP)
func Login(tiqn, portal, iface string) error {
	args := []string{"-m", "node", "-T", tiqn, "-p", portal}
	loginArgs := append(args, []string{"--login"}...)
	Trace.Printf("attempt login with args: %s", loginArgs)
	_, err := exec.Command("iscsiadm", loginArgs...).CombinedOutput()
	return err
}

// GetDevice Attempts to gather device info for the specified target.
// If the device file is not found, we assuem the target is not connected
// and return an empty Device struct
func GetDevice(tgtIQN string) (Device, error) {
	args := []string{"-t"}
	out, err := exec.Command("lsscsi", args...).CombinedOutput()
	if err != nil {
		Error.Printf("unable to perform lsscsi -t, error: %+v", err)
		return Device{}, err
	}
	devices := strings.Split(strings.TrimSpace(string(out)), "\n")
	dev := Device{}
	for _, entry := range devices {
		if strings.Contains(entry, tgtIQN) {
			fields := strings.Fields(entry)
			dev.Path = fields[len(fields)-1]
		}
	}
	Info.Printf("found lsscsi device: %s\n", dev)

	if isMultipath(dev.Path) == true {
		Info.Println("multipath detected...")
		args = []string{dev.Path, "-n", "-o", "name", "-r"}
		out, err = exec.Command("lsblk", args...).CombinedOutput()
		if err != nil {
			Error.Printf("unable to find mpath device due to lsblk error: %+v\n", err)
			return dev, err
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 1 {
			mpdev := lines[1]
			dev.MPDevice = "/dev/mapper/" + mpdev
			Info.Printf("parsed %s to extract mp device: %s\n", lines, dev.MPDevice)

		} else {
			Error.Printf("unable to parse lsblk output (%v)\n", lines)
			// FIXME(jdg): Create an error here and return it
		}

	}
	return dev, nil
}
