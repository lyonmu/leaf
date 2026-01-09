package config

import (
	"github.com/lyonmu/leaf/pkg/config"
	"github.com/lyonmu/leaf/pkg/logger"
)

type Config struct {
	Version bool               `short:"v" long:"version" help:"版本信息" default:"false" mapstructure:"version" json:"version" yaml:"version"`
	Log     logger.LogConfig   `embed:"" prefix:"log." mapstructure:"log" json:"log" yaml:"log"`
	Gateway LeafConfig         `embed:"" prefix:"leaf." mapstructure:"leaf" json:"leaf" yaml:"leaf"`
	Kafka   config.KafkaConfig `embed:"" prefix:"kafka." mapstructure:"kafka" json:"kafka" yaml:"kafka"`
}

type LeafConfig struct {
	Node int `name:"node" env:"NODE" default:"1" help:"节点编号" mapstructure:"node" yaml:"node" json:"node"`
}

func (c *Config) MachineID() (int, error) {
	return c.Gateway.Node, nil
}
