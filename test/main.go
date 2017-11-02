package main

import (
	"log"

	"github.com/j-griffith/goiscsi"
)

func main() {
	inf := &iscsi.Info
	device := iscsi.Device{}
	device.Portal = "10.117.37.101:3260"
	device.IFace = "default"
	device.IQN = "iqn.2010-01.com.solidfire:6z9n.foo.22952"
	//path, _ := iscsi.Attach(&device)
	//fmt.Printf("path is: %+v", path)
	dev, err := iscsi.GetDevice(device.IQN)
	if err != nil {
		log.Printf("err response: %s", err)
	} else {
		log.Printf("returned device: %v", dev)
	}

}
