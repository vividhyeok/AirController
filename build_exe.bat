pyinstaller --noconfirm --onefile --console --name "AirController" --add-data "templates;templates" --hidden-import=eventlet --hidden-import=engineio.async_drivers.eventlet --hidden-import=engineio.async_drivers.threading remote_server.py
pause
