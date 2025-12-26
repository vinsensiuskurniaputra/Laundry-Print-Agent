package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	PORT      = ":3491"
	API_TOKEN = "CLEANLINK_SECRET_123"
)

// ================= STRUCT =================

type QRCodeData struct {
	ServiceName string `json:"service_name"`
	OrderID     string `json:"order_id"`
	Body        string `json:"body"`
	QRValue     string `json:"qr_value"`
}

type PrintRequest struct {
	Token      string       `json:"token"`
	Title      string       `json:"title"`
	OrderID    string       `json:"order_id"`
	Body       string       `json:"body"`
	QRValue    string       `json:"qr_value"`     // Deprecated: use QRCodes array instead
	PrintMode  string       `json:"print_mode"`   // "all", "receipt-only", "qr-only"
	QRCodes    []QRCodeData `json:"qr_codes"`     // Array of QR codes to print
	NoPaperCut bool         `json:"no_paper_cut"` // Deprecated: not needed anymore
}

// ================= MAIN =================

func main() {
	http.HandleFunc("/ping", corsMiddleware(ping))
	http.HandleFunc("/print", corsMiddleware(print))
	http.HandleFunc("/check", corsMiddleware(checkPrinter))

	fmt.Println("Cleanlink Printer Agent running on", PORT)
	http.ListenAndServe(PORT, nil)
}

// ================= CORS MIDDLEWARE =================

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call the next handler
		next(w, r)
	}
}

// ================= HANDLERS =================

func ping(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func checkPrinter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if runtime.GOOS != "windows" {
		w.Write([]byte(`{"status":"error","message":"Windows only"}`))
		return
	}

	// Try to detect COM port
	com, err := detectPrinterCOM()
	if err != nil {
		w.Write([]byte(`{"status":"error","message":"` + err.Error() + `","com":""}`))
		return
	}

	// Try to open the port to verify it's accessible
	port := `\\.\` + com
	f, err := os.OpenFile(port, os.O_WRONLY, 0)
	if err != nil {
		w.Write([]byte(`{"status":"error","message":"Cannot access COM port: ` + err.Error() + `","com":"` + com + `"}`))
		return
	}
	f.Close()

	w.Write([]byte(`{"status":"ok","message":"Printer COM port detected and accessible","com":"` + com + `"}`))
}

func print(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req PrintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	if req.Token != API_TOKEN {
		http.Error(w, "Unauthorized", 401)
		return
	}

	com, err := detectPrinterCOM()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	receipt := []byte{}

	// Initialize printer
	receipt = append(receipt, escInit()...)

	// Determine print mode
	printMode := req.PrintMode
	if printMode == "" {
		// Default: if QR codes exist, print all; otherwise receipt only
		if len(req.QRCodes) > 0 {
			printMode = "all"
		} else if req.QRValue != "" {
			// Backward compatibility: single QR value
			printMode = "all"
		} else {
			printMode = "receipt-only"
		}
	}

	// RECEIPT-ONLY MODE: Print only receipt
	if printMode == "receipt-only" {
		// Top spacing
		receipt = append(receipt, []byte("\n")...)

		// Header - Branch name
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, escDoubleHeight(true)...)
		receipt = append(receipt, []byte(req.Title+"\n")...)
		receipt = append(receipt, escDoubleHeight(false)...)
		receipt = append(receipt, escBold(false)...)

		// Separator line (32 chars for 57mm paper)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("================================\n")...)

		// Order ID - Centered and Bold
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("ORDER: "+req.OrderID+"\n")...)
		receipt = append(receipt, escBold(false)...)

		// Separator
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)

		// Body content - Left aligned
		receipt = append(receipt, escAlignLeft()...)
		receipt = append(receipt, []byte(req.Body)...)

		// Bottom spacing and thank you message
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("\n================================\n")...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("Terima kasih\n")...)
		receipt = append(receipt, escBold(false)...)
		receipt = append(receipt, []byte("Atas kepercayaan Anda\n")...)

		// Bottom spacing before cut
		receipt = append(receipt, []byte("\n\n")...)
		receipt = append(receipt, escCut()...)
	} else if printMode == "qr-only" {
		// QR-ONLY MODE: Print only QR code labels
		// Print all QR codes from the array
		for i, qrData := range req.QRCodes {
			if i > 0 {
				// Add spacing between QR codes
				receipt = append(receipt, []byte("\n\n")...)
			}

			// Header - Service name
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, escDoubleHeight(true)...)
			receipt = append(receipt, []byte(qrData.ServiceName+"\n")...)
			receipt = append(receipt, escDoubleHeight(false)...)
			receipt = append(receipt, escBold(false)...)

			// Separator line
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("================================\n")...)

			// Order ID - Centered and Bold
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("ORDER: "+qrData.OrderID+"\n")...)
			receipt = append(receipt, escBold(false)...)

			// Separator
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)

			// Body content - Left aligned
			receipt = append(receipt, escAlignLeft()...)
			receipt = append(receipt, []byte(qrData.Body)...)

			// Separator before QR code
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("\n--------------------------------\n")...)

			// QR CODE - Centered
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escQRCode(qrData.QRValue)...)

			// Text below QR code
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("Scan untuk update status\n")...)
			receipt = append(receipt, escBold(false)...)

			// Bottom spacing before cut (only after last QR code)
			if i == len(req.QRCodes)-1 {
				receipt = append(receipt, []byte("\n\n")...)
				receipt = append(receipt, escCut()...)
			}
		}

		// Backward compatibility: single QR value
		if len(req.QRCodes) == 0 && req.QRValue != "" {
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, escDoubleHeight(true)...)
			receipt = append(receipt, []byte(req.Title+"\n")...)
			receipt = append(receipt, escDoubleHeight(false)...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("================================\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("ORDER: "+req.OrderID+"\n")...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)
			receipt = append(receipt, escAlignLeft()...)
			receipt = append(receipt, []byte(req.Body)...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("\n--------------------------------\n")...)
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escQRCode(req.QRValue)...)
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("Scan untuk update status\n")...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escCut()...)
		}
	} else {
		// ALL MODE: Print receipt, separator, then all QR codes
		// 1. Print receipt
		receipt = append(receipt, []byte("\n")...)

		// Header - Branch name
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, escDoubleHeight(true)...)
		receipt = append(receipt, []byte(req.Title+"\n")...)
		receipt = append(receipt, escDoubleHeight(false)...)
		receipt = append(receipt, escBold(false)...)

		// Separator line
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("================================\n")...)

		// Order ID - Centered and Bold
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("ORDER: "+req.OrderID+"\n")...)
		receipt = append(receipt, escBold(false)...)

		// Separator
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)

		// Body content - Left aligned
		receipt = append(receipt, escAlignLeft()...)
		receipt = append(receipt, []byte(req.Body)...)

		// Bottom spacing and thank you message
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("\n================================\n")...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("Terima kasih\n")...)
		receipt = append(receipt, escBold(false)...)
		receipt = append(receipt, []byte("Atas kepercayaan Anda\n")...)

		// 2. Print separator barrier
		receipt = append(receipt, []byte("\n\n")...)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("--- Untuk Staff ---\n")...)
		receipt = append(receipt, escBold(false)...)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)
		receipt = append(receipt, []byte("\n\n")...)

		// 3. Print all QR codes
		for i, qrData := range req.QRCodes {
			if i > 0 {
				// Add spacing between QR codes
				receipt = append(receipt, []byte("\n\n")...)
			}

			// Header - Service name
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, escDoubleHeight(true)...)
			receipt = append(receipt, []byte(qrData.ServiceName+"\n")...)
			receipt = append(receipt, escDoubleHeight(false)...)
			receipt = append(receipt, escBold(false)...)

			// Separator line
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("================================\n")...)

			// Order ID - Centered and Bold
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("ORDER: "+qrData.OrderID+"\n")...)
			receipt = append(receipt, escBold(false)...)

			// Separator
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)

			// Body content - Left aligned
			receipt = append(receipt, escAlignLeft()...)
			receipt = append(receipt, []byte(qrData.Body)...)

			// Separator before QR code
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("\n--------------------------------\n")...)

			// QR CODE - Centered
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escQRCode(qrData.QRValue)...)

			// Text below QR code
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("Scan untuk update status\n")...)
			receipt = append(receipt, escBold(false)...)

			// Cut paper only after last QR code
			if i == len(req.QRCodes)-1 {
				receipt = append(receipt, []byte("\n\n")...)
				receipt = append(receipt, escCut()...)
			}
		}

		// Backward compatibility: single QR value
		if len(req.QRCodes) == 0 && req.QRValue != "" {
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, escDoubleHeight(true)...)
			receipt = append(receipt, []byte(req.Title+"\n")...)
			receipt = append(receipt, escDoubleHeight(false)...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("================================\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("ORDER: "+req.OrderID+"\n")...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)
			receipt = append(receipt, escAlignLeft()...)
			receipt = append(receipt, []byte(req.Body)...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("\n--------------------------------\n")...)
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escQRCode(req.QRValue)...)
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("Scan untuk update status\n")...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escCut()...)
		}
	}

	err = writeToCOM(com, receipt)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"printed","com":"` + com + `"}`))
}

// ================= CORE =================

func detectPrinterCOM() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("Windows only")
	}

	// Create context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use Get-CimInstance instead of Get-WmiObject (more modern and faster)
	// Also filter for Bluetooth devices specifically
	cmd := exec.CommandContext(
		ctx,
		"powershell",
		"-NoProfile",
		"-Command",
		`Get-CimInstance Win32_SerialPort | Where-Object { $_.Name -like "*Bluetooth*" } | Select-Object -ExpandProperty DeviceID`,
	)

	out, err := cmd.Output()
	if err != nil {
		// If command fails, try alternative method: check common COM ports
		return detectPrinterCOMAlternative()
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return detectPrinterCOMAlternative()
	}

	// Extract COM port from DeviceID (format: COM10)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if line contains COM
		if strings.HasPrefix(strings.ToUpper(line), "COM") {
			return line, nil
		}

		// Try to extract COM from DeviceID format
		start := strings.Index(strings.ToUpper(line), "COM")
		if start != -1 {
			end := start + 3 // COM
			for end < len(line) && (line[end] >= '0' && line[end] <= '9') {
				end++
			}
			if end > start+3 {
				return line[start:end], nil
			}
		}
	}

	// Fallback to alternative detection
	return detectPrinterCOMAlternative()
}

func detectPrinterCOMAlternative() (string, error) {
	// Try common COM ports for Bluetooth printers
	// Based on the image, the user's printer is on COM10, so check it first
	commonPorts := []string{"COM10", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM11", "COM12"}

	for _, port := range commonPorts {
		testPort := `\\.\` + port
		f, err := os.OpenFile(testPort, os.O_WRONLY, 0)
		if err == nil {
			f.Close()
			return port, nil
		}
	}

	return "", errors.New("Bluetooth printer COM port not found. Please check if printer is connected. Based on your config, try COM10")
}

func writeToCOM(com string, data []byte) error {
	port := `\\.\` + com

	// Open port and write all data at once to avoid delays
	// Writing all data in a single operation prevents delays between commands
	f, err := os.OpenFile(port, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write all data in one operation to prevent delays
	// This ensures all ESC/POS commands are sent together without buffering delays
	_, err = f.Write(data)
	return err
}

// ================= ESC/POS FUNCTIONS =================

func escInit() []byte {
	return []byte{0x1B, 0x40}
}

func escBold(on bool) []byte {
	if on {
		return []byte{0x1B, 0x45, 0x01}
	}
	return []byte{0x1B, 0x45, 0x00}
}

func escAlignCenter() []byte {
	return []byte{0x1B, 0x61, 0x01}
}

func escAlignLeft() []byte {
	return []byte{0x1B, 0x61, 0x00}
}

func escCut() []byte {
	return []byte{0x1D, 0x56, 0x00}
}

func escDoubleHeight(on bool) []byte {
	if on {
		return []byte{0x1B, 0x21, 0x10} // Double height
	}
	return []byte{0x1B, 0x21, 0x00} // Normal height
}

func escUnderline(on bool) []byte {
	if on {
		return []byte{0x1B, 0x2D, 0x01} // Underline on
	}
	return []byte{0x1B, 0x2D, 0x00} // Underline off
}

func escFontSize(normal bool) []byte {
	if normal {
		return []byte{0x1B, 0x21, 0x00} // Normal font
	}
	return []byte{0x1B, 0x21, 0x01} // Small font
}

// ================= QR CODE =================

func escQRCode(data string) []byte {
	cmd := []byte{}

	// Model 2 (recommended for most printers)
	cmd = append(cmd, []byte{0x1D, 0x28, 0x6B, 0x04, 0x00, 0x31, 0x41, 0x32, 0x00}...)

	// Size (7) - Optimized for 57mm paper (good balance between size and scannability)
	// Size range: 1-16
	// For 57mm paper: 6-8 is recommended, 7 is optimal
	// If QR code is too small, increase to 8. If too large, decrease to 6.
	cmd = append(cmd, []byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x43, 0x07}...)

	// Error correction level (M - Medium) for better reliability
	// 0x30 = L (Low), 0x31 = M (Medium), 0x32 = Q (Quartile), 0x33 = H (High)
	cmd = append(cmd, []byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x45, 0x31}...)

	// Store data
	dataBytes := []byte(data)
	pL := byte(len(dataBytes) + 3)
	pH := byte(0x00)
	if len(dataBytes)+3 > 255 {
		pH = byte((len(dataBytes) + 3) / 256)
		pL = byte((len(dataBytes) + 3) % 256)
	}

	cmd = append(cmd,
		0x1D, 0x28, 0x6B,
		pL, pH,
		0x31, 0x50, 0x30,
	)
	cmd = append(cmd, dataBytes...)

	// Print QR code
	cmd = append(cmd, []byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x51, 0x30}...)

	return cmd
}
