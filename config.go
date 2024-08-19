package main

import (
	"github.com/spf13/viper"
	"log"
)

type ChatClientConfig struct {
	ServerHost string
	AuthToken  string
}

var config ChatClientConfig

func InitConfig() {
	viper.SetConfigFile(".env")

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Fatalf("Error reading config file, %s", err)
		}
	}

	viper.AutomaticEnv()

	viper.SetEnvPrefix("CHAT_CLIENT_")

	viper.BindEnv("SERVER_HOST")
	viper.BindEnv("AUTH_TOKEN")

	config.ServerHost = viper.GetString("SERVER_HOST")
	if config.ServerHost == "" {
		log.Fatal("CHAT_CLIENT_SERVER_HOST is not set in the .env file or environment variables")
	}

	config.AuthToken = viper.GetString("AUTH_TOKEN")

	if config.AuthToken == "" {
		log.Fatal("CHAT_CLIENT_AUTH_TOKEN is not set in the .env file or environment variables")
	}
}

func GetConfig() *ChatClientConfig {
	return &config
}
