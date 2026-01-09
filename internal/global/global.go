package global

import (
	"github.com/lyonmu/leaf/internal/config"
	"github.com/lyonmu/leaf/pkg/tools"
	"go.uber.org/zap"
)

var (
	Cfg    config.Config
	Logger *zap.Logger
	Id     tools.IDGenerator
)
