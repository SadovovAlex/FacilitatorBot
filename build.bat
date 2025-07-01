@echo off
chcp 65001 > nul  :: Устанавливаем UTF-8 для корректного отображения кириллицы
cls

:: Завершаем процесс facilitatorbot.exe (если запущен)
echo Завершение работающего процесса facilitatorbot.exe...
taskkill /IM "facilitatorbot.exe" /F >nul 2>&1
if %errorlevel% equ 0 (
    echo [OK] Процесс facilitatorbot.exe был завершен.
) else (
    echo [INFO] Процесс facilitatorbot.exe не найден или уже завершен.
)

:: Получаем текущую дату и время в формате RFC3339
for /f "tokens=1-3 delims=/" %%a in ("%date%") do (
    set day=%%a
    set month=%%b
    set year=%%c
)
set BUILD_DATE=%year%-%month%-%day%T%time:~0,2%:%time:~3,2%:%time:~6,2%Z

:: Собираем проект
echo Сборка проекта...
go build -ldflags "-X main.BuildDate=%BUILD_DATE%" ./...

:: Проверяем успешность сборки
if %errorlevel% neq 0 (
    echo [ОШИБКА] Сборка не удалась! Код ошибки: %errorlevel%
    pause
    exit /b %errorlevel%
)

:: Запускаем программу
echo [OK] Сборка успешно завершена, запускаем программу...
start "" "facilitatorbot.exe"

pause