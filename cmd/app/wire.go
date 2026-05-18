//go:build wireinject
// +build wireinject

package main

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"github.com/makesalekz/platform-billing/internal/biz"
	"github.com/makesalekz/platform-billing/internal/conf"
	"github.com/makesalekz/platform-billing/internal/data"
	"github.com/makesalekz/platform-billing/internal/server"
	"github.com/makesalekz/platform-billing/internal/service"
)

func wireApp(*conf.Bootstrap, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, data.ProviderSet, biz.ProviderSet, service.ProviderSet, newApp))
}
