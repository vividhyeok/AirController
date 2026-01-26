pyinstaller --noconfirm --onefile --console --name "AirController" --add-data "templates;templates" --hidden-import=eventlet --hidden-import=engineio.async_drivers.eventlet remote_server.py
pause