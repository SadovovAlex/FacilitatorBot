@echo off

:: Получаем текущую дату и время в формате RFC3339
set BUILD_DATE=%date:~6,4%-%date:~3,2%-%date:~0,2%T%time:~0,2%:%time:~3,2%:%time:~6,2%Z

:: Собираем бота с информацией о версии и дате сборки
go build -ldflags "-X main.BuildDate=%BUILD_DATE%" ./...

facilitatorbot.exe

pause
