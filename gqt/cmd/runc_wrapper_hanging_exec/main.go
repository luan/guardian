package main

import (
	"os"
	"os/exec"

	"github.com/Sirupsen/logrus"
)

func main() {
	logPath := ""
	isExec := false

	for idx, s := range os.Args {
		if s == "-log" || s == "--log" {
			logPath = os.Args[idx+1]
		}
		if s == "exec" {
			isExec = true
		}
	}

	f, err := os.Create(logPath)
	if err != nil {
		os.Exit(1)
	}

	logrus.SetOutput(f)

	if isExec {
		logrus.Warn("guardian-runc-about-to-hang")

		syncPipePath := os.Getenv("TEST_RUNC_PIPE")
		syncPipe, err := os.OpenFile(syncPipePath, os.O_RDONLY, 0600)
		if err != nil {
			panic(err)
		}

		syncPipe.Read(make([]byte, 1))
	}

	cmd := exec.Command("runc", os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		panic(err)
	}
}
