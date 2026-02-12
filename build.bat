@echo off
echo [1/3] Cleaning previous builds...
if exist dist del /q dist\*
if exist build rd /s /q build

echo [2/3] Building executable with PyInstaller...
python -m PyInstaller --noconsole --onefile --add-data "templates;templates" remote_server.py

echo [3/3] Build complete. Checking dist folder...
dir dist
