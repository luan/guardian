package main

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/urfave/cli"
)

func main() {
	fakeImagePlugin := cli.NewApp()
	fakeImagePlugin.Name = "fakeImagePlugin"
	fakeImagePlugin.Usage = "I am FakeImagePlugin!"

	fakeImagePlugin.Flags = []cli.Flag{
		cli.BoolFlag{
			Name: "debug",
		},
		cli.StringFlag{
			Name: "log",
		},
		cli.StringFlag{
			Name: "newuidmap",
		},
		cli.StringFlag{
			Name: "newgidmap",
		},
	}

	fakeImagePlugin.Commands = []cli.Command{
		CreateCommand,
	}

	_ = fakeImagePlugin.Run(os.Args)
}

var CreateCommand = cli.Command{
	Name: "create",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name: "no-new-keyring",
		},
		cli.StringFlag{
			Name: "bundle",
		},
		cli.StringFlag{
			Name: "pid-file",
		},
	},

	Action: func(ctx *cli.Context) error {
		err := ioutil.WriteFile("/tmp/args", []byte(strings.Join(os.Args, " ")), 0777)
		if err != nil {
			panic(err)
		}

		return nil
	},
}
