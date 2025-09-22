@echo off
SETLOCAL EnableDelayedExpansion

:: Создание директории dist если ее нет
if not exist "dist\" (
    echo [INFO] Creating dist directory...
    mkdir dist
    if errorlevel 1 (
        echo [ERR] Failed to create dist directory
        pause
        exit /b 1
    )
)

:: Функция для увеличения версии
call :IncrementVersion
if errorlevel 1 (
    echo [ERR] Failed to increment version
    pause
    exit /b 1
)

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

:: Очистка предыдущей сборки в dist
if exist "dist\facilitatorbot.exe" (
    echo [INFO] Removing previous build from dist...
    del /F /Q "dist\facilitatorbot.exe" >nul 2>&1
    if errorlevel 1 (
        echo [ERR] Failed to remove previous executable from dist
        pause
        exit /b 1
    )
)

echo [INFO] Building version !NEW_VERSION! with date: %BUILD_DATE%
go build -ldflags "-X main.BuildDate=%BUILD_DATE% -X main.Version=!NEW_VERSION!" -o "dist\facilitatorbot.exe"

if %errorlevel% neq 0 (
    echo [ERR] Build failed with error: %errorlevel%
    pause
    exit /b %errorlevel%
)

:: Проверка что файл создан в dist
if not exist "dist\facilitatorbot.exe" (
    echo [ERR] Executable was not created in dist directory after successful build
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

:: Запуск приложения из dist
echo [OK] Build successful v!NEW_VERSION!, starting application from dist...
cd dist/
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

:IncrementVersion
:: Чтение текущей версии из version.go
set "version_file=version.go"
if not exist "%version_file%" (
    echo [ERR] Version file %version_file% not found
    exit /b 1
)

:: Выполняем поиск строки с Version и выводим результат
echo [INFO] Searching for Version in %version_file%...
findstr "Version" "%version_file%"
set "search_result=%errorlevel%"

:: Сохраняем найденную строку в переменную
for /f "delims=" %%i in ('findstr "Version" "%version_file%"') do (
    set "version_line=%%i"
    echo [INFO] Found line: %%i
)

if "!version_line!"=="" (
    echo [ERR] Version line not found in %version_file%
    echo [INFO] File content:
    type "%version_file%"
    exit /b 1
)

:: Извлекаем версию из найденной строки
:: Удаляем все до первой кавычки
set "temp=!version_line:*"=!"
:: Удаляем все после второй кавычки
set "current_version=!temp:"=!"


if "!current_version!"=="" (
    echo [ERR] Could not extract version from line: !version_line!
    exit /b 1
)

echo [INFO] Current version: !current_version!

:: Разбор версии на компоненты
for /f "tokens=1-3 delims=." %%a in ("!current_version!") do (
    set "major=%%a"
    set "minor=%%b"
    set "patch=%%c"
)

:: Проверка что все компоненты версии извлечены
if "!major!"=="" (
    echo [ERR] Invalid version format: !current_version!
    exit /b 1
)

if "!minor!"=="" set "minor=0"
if "!patch!"=="" set "patch=0"

:: Увеличение patch версии
set /a new_patch=patch + 1
set "NEW_VERSION=!major!.!minor!.!new_patch!"

echo [INFO] New version: !NEW_VERSION!

:: Обновление версии в файле
echo [INFO] Updating version in %version_file%...
powershell -Command "$content = Get-Content '%version_file%'; $newContent = $content -replace '!current_version!', '!NEW_VERSION!'; Set-Content '%version_file%' $newContent"

if errorlevel 1 (
    echo [ERR] Failed to update version in %version_file%
    exit /b 1
)

echo [OK] Version updated successfully from !current_version! to !NEW_VERSION!
exit /b 0