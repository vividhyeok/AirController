@echo off
echo Building AirController.exe...
pyinstaller --noconfirm AirController.spec
echo.
echo Build complete! Check dist folder for AirController.exe
pause
