package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/lyonmu/leaf/internal/common"
	"github.com/lyonmu/leaf/internal/bootstrap"
	"github.com/lyonmu/leaf/internal/global"
	"github.com/prometheus/common/version"
)

func main() {
	kong.Parse(&global.Cfg,
		kong.Name(common.ProjectName),
		kong.Description(common.ProjectName),
		kong.UsageOnError(),
		kong.HelpOptions{
			Compact: true,
			Summary: true,
		},
	)
	if global.Cfg.Version {
		fmt.Println(version.Print(common.ProjectName))
		os.Exit(0)
	}

	bootstrap.Start()
}
