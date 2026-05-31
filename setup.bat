@echo off
chcp 65001 >nul
echo === Этап 10: Установка зависимостей (fsnotify) ===
echo.

:: Умная проверка директории (ищем go.mod)
if not exist "go.mod" (
    if exist "pkm-daemon\go.mod" (
        echo [INFO] Найдена папка pkm-daemon. Переходим в нее...
        cd pkm-daemon
    ) else (
        echo [ОШИБКА] Файл go.mod не найден! 
        echo Убедитесь, что скрипт лежит в корневой папке проекта.
        pause
        exit /b 1
    )
) else (
    echo [INFO] Директория корректна.
)

echo.
echo [INFO] Скачиваем библиотеку fsnotify для отслеживания ФС...
go get github.com/fsnotify/fsnotify

echo [INFO] Очистка и синхронизация зависимостей (go mod tidy)...
go mod tidy

echo.
echo [УСПЕХ] Зависимости для Этапа 10 успешно установлены!
pause