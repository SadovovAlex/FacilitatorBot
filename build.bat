@echo off
SETLOCAL EnableDelayedExpansion

:: Получение даты и времени без WMIC (универсальный способ)
for /f "tokens=1-3 delims=/" %%a in ('echo %date%') do (
    set _day=%%a
)

:: Удаление возможных пробелов в значениях даты
set _day=%_day: =%

:: Получение времени (обработка форматов с AM/PM)
set _time=%time%
if "%_time:~0,1%"==" " set _time=0%_time:~1%

:: Форматирование времени
set _hour=%_time:~0,2%
set _minute=%_time:~3,2%
set _second=%_time:~6,2%

:: Создание строки с датой/временем в ISO формате
set BUILD_DATE=%_day%T%_hour%:%_minute%:%_second%Z

:: Создание строки с датой/временем в ISO формате
set BUILD_DATE=%_day%T%_hour%:%_minute%:%_second%Z

:: Очистка предыдущей сборки
if exist "facilitatorbot.exe" (
    echo [INFO] Removing previous build...
    del /F /Q facilitatorbot.exe >nul 2>&1
    if errorlevel 1 (
        echo [ERR] Failed to remove previous executable
        pause
        exit /b 1
    )
)

echo [INFO] Building with date: %BUILD_DATE%
go build -ldflags "-X main.BuildDate=%BUILD_DATE%" -o facilitatorbot.exe

if %errorlevel% neq 0 (
    echo [ERR] Build failed with error: %errorlevel%
    pause
    exit /b %errorlevel%
)

:: Проверка что файл создан
if not exist "facilitatorbot.exe" (
    echo [ERR] Executable was not created after successful build
    pause
    exit /b 1
)

:: Проверка запущенного процесса
tasklist /FI "IMAGENAME eq facilitatorbot.exe" 2>NUL | find /I "facilitatorbot.exe" >NUL
if %errorlevel% equ 0 (
    echo [INFO] Application is already running, killing existing process...
    taskkill /F /IM "facilitatorbot.exe" >nul 2>&1
    timeout /t 2 >nul
)

:: Запуск приложения
echo [OK] Build successful, starting application...
start "" "facilitatorbot.exe"

:: Проверка что процесс запустился
timeout /t 2 >nul
tasklist /FI "IMAGENAME eq facilitatorbot.exe" >nul && (
    echo [OK] Application started successfully.
) || (
    echo [ERR] Failed to start the application.
    pause
    exit /b 1
)

exit /b 0