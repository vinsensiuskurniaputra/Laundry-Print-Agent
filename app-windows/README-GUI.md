# Cleanlink Printer Agent - GUI Version

## Building Instructions

### For Windows (on Windows machine):
```bash
go build -ldflags="-H windowsgui" -o cleanlink-printer-gui.exe ./app-windows
```

### For Windows (cross-compile from Linux - requires CGO):
```bash
# Install dependencies first
sudo apt-get install gcc-mingw-w64-x86-64

# Build with CGO
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o cleanlink-printer-gui.exe ./app-windows
```

## Features

- **Graphical User Interface**: Easy-to-use GUI for printer selection
- **Printer Detection**: Automatically detects all available serial printers (Bluetooth and USB)
- **Printer Selection**: Choose which printer to use from dropdown
- **Server Status**: Visual feedback on server status
- **Test Print**: Test your printer connection before use
- **System Tray Support**: Minimizes to system tray (optional)

## Usage

1. Launch the application (cleanlink-printer-gui.exe)
2. Click "Detect Printers" to scan for available printers
3. Select your printer from the dropdown list
4. Click "Start Server" to begin accepting print jobs
5. Use "Test Print" to verify the connection

The server will run on `http://localhost:3491` and use the selected printer for all print jobs.
