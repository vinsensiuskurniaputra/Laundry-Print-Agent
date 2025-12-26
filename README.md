# Cleanlink Printer Agent - Complete Guide

This application provides two interfaces for selecting and using your thermal printer:

## 🎨 GUI Version (main.go)
Beautiful graphical interface with printer selection

## 💻 Console Version (main-console.go)  
Simple text-based interface, easier to build

---

## Features

### ✨ New Features
- **Multi-Printer Support**: Detects ALL serial printers (Bluetooth AND USB)
- **Printer Selection**: Choose which printer to use before starting server
- **Test Print**: Verify printer connection before going live
- **User-Friendly**: No need to manually configure COM ports

### 🖨️ Printing Features
- Multiple print modes: `all`, `receipt-only`, `qr-only`
- Multiple QR codes per receipt
- ESC/POS thermal printer support
- Auto paper cut
- 57mm thermal paper optimized

---

## 📦 Building

### Console Version (Recommended for cross-compile)
```bash
# From Linux to Windows
GOOS=windows GOARCH=amd64 go build -o cleanlink-printer-console.exe ./app-windows/main-console.go

# On Windows
go build -o cleanlink-printer-console.exe ./app-windows/main-console.go
```

### GUI Version
```bash
# On Windows (easiest)
go build -ldflags="-H windowsgui" -o cleanlink-printer-gui.exe ./app-windows

# From Linux (requires CGO and mingw)
sudo apt-get install gcc-mingw-w64-x86-64
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o cleanlink-printer-gui.exe ./app-windows
```

---

## 🚀 Usage

### Console Version

1. **Run the executable**
   ```
   cleanlink-printer-console.exe
   ```

2. **The app will:**
   - Detect all available printers
   - Show you a list to choose from
   - Let you test the printer
   - Start the HTTP server

3. **Example flow:**
   ```
   ========================================
      Cleanlink Printer Agent - Console
   ========================================

   🔍 Detecting printers...

   📋 Available Printers:
   ----------------------------------------
   1. Standard Serial over Bluetooth link (COM10)
   2. USB Serial Port (COM5)
   ----------------------------------------

   👉 Select printer number (1-2): 1

   ✅ Selected Printer: Standard Serial over Bluetooth link
      Port: COM10

   🧪 Would you like to test the printer? (y/n): y
   📄 Sending test print...
   ✅ Test print completed successfully!

   🚀 Starting HTTP server...

   ✅ Server is running!
   ========================================
   📡 Endpoint: http://localhost:3491
   🖨️  Printer: COM10
   ========================================

   Press Ctrl+C to stop the server
   ```

### GUI Version

1. **Launch** `cleanlink-printer-gui.exe`
2. **Click** "Detect Printers"
3. **Select** your printer from dropdown
4. **Click** "Test Print" (optional)
5. **Click** "Start Server"

---

## 🔌 API Endpoints

### 1. **Ping** - Check server status
```
GET http://localhost:3491/ping
```

### 2. **Print** - Send print job
```
POST http://localhost:3491/print
Content-Type: application/json

{
  "token": "CLEANLINK_SECRET_123",
  "title": "Cleanlink Laundry",
  "order_id": "ORD-12345",
  "body": "Item: Shirt\nPrice: Rp 10,000\n",
  "print_mode": "receipt-only",
  "qr_codes": [
    {
      "service_name": "Cuci + Setrika",
      "order_id": "ORD-12345-1",
      "body": "1x Kemeja Putih\n",
      "qr_value": "https://cleanlink.com/track/ORD-12345-1"
    }
  ]
}
```

**Print Modes:**
- `"receipt-only"` - Receipt only (customer copy)
- `"qr-only"` - QR code labels only (staff copy)
- `"all"` - Receipt + QR labels (default if qr_codes exist)

### 3. **Check** - Verify printer status
```
GET http://localhost:3491/check
```

---

## 🔧 How Printer Detection Works

### Primary Method (PowerShell)
Queries Windows Management Instrumentation (WMI) for serial ports:
```powershell
Get-CimInstance Win32_SerialPort | Select-Object Name, DeviceID
```

### Fallback Method
Tests common COM ports (COM1-COM12) by attempting to open them.

### Supported Printers
- ✅ Bluetooth thermal printers (via COM port)
- ✅ USB thermal printers (via USB-to-Serial, appears as COM port)
- ✅ Any ESC/POS compatible thermal printer on serial port

---

## 📝 Configuration

### Change Port
Edit `const PORT` in the source code:
```go
const PORT = ":3491"  // Change to your desired port
```

### Change API Token
Edit `const API_TOKEN` in the source code:
```go
const API_TOKEN = "CLEANLINK_SECRET_123"  // Your secret token
```

---

## 🐛 Troubleshooting

### No Printers Found
1. **Check if printer is powered on**
2. **Check if printer is paired** (for Bluetooth)
3. **Check Device Manager** → Ports (COM & LPT) - ensure COM port is shown
4. **Run as Administrator** - some COM ports need elevated privileges

### Can't Access COM Port
- **Close other programs** that might be using the printer
- **Restart printer** and reconnect
- **Check permissions** - run as Administrator

### Print Quality Issues
- Adjust QR code size in `escQRCode()` function (currently set to 7)
- Check paper alignment
- Ensure paper is 57mm thermal paper

---

## 📂 Project Structure

```
cleanlink-printer-agent/
├── app-windows/
│   ├── main.go              # GUI version (Fyne)
│   ├── main-console.go      # Console version
│   └── README-GUI.md        # GUI-specific docs
├── go.mod
├── go.sum
└── README.md                # This file
```

---

## 🎯 Differences Between Versions

| Feature | GUI Version | Console Version |
|---------|-------------|-----------------|
| Interface | Graphical window | Text-based |
| Cross-compile | Requires CGO | ✅ Easy |
| User experience | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Resource usage | Higher | Lower |
| System tray | ✅ Yes | ❌ No |
| Best for | Desktop users | Server/Kiosk |

---

## 🚀 Deployment Recommendations

### For Desktop/POS Station
Use **GUI version** for better user experience

### For Server/Kiosk Mode
Use **Console version** and create a Windows shortcut that auto-starts on login

### Auto-Start on Windows
1. Press `Win + R`
2. Type `shell:startup`
3. Create shortcut to the executable in this folder

---

## 📄 License

Proprietary - Cleanlink Laundry System

---

## 👨‍💻 Support

For issues or questions, contact your system administrator.
