#!/bin/bash

# Получаем текущую дату в формате RFC3339
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Собираем бота с информацией о версии и дате сборки
go build -ldflags "-X main.BuildDate=${BUILD_DATE}" ./...
