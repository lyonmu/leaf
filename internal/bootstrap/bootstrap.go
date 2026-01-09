package bootstrap

import (
	"os"
	"sync"

	"github.com/lyonmu/leaf/internal/global"
	"github.com/lyonmu/leaf/pkg/logger"
	"github.com/lyonmu/leaf/pkg/tools"
	"gopkg.in/yaml.v3"
)

var once sync.Once

func Start() {

	once.Do(func() {
		global.Logger = logger.NewZapLogger(global.Cfg.Log)
		global.Logger.Sugar().Info("服务初始化开始")

		bytes, _ := yaml.Marshal(global.Cfg)
		global.Logger.Sugar().Infof("==================== launching leaf with config ====================\n%s", string(bytes))

		idGenerator, err := tools.NewSonySnowFlake(global.Cfg.MachineID)
		if err != nil {
			global.Logger.Sugar().Error("初始化雪花算法失败: %v", err)
			os.Exit(1)
		}
		global.Id = idGenerator

	})
}
