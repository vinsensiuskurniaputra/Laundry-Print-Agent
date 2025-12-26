package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// ================= CONFIG =================

const (
	PORT        = ":3491"
	PRINTER_DEV = "/dev/rfcomm0" // Linux Bluetooth / USB serial
	API_TOKEN   = "CLEANLINK_SECRET_123"
)

// ================= STRUCT =================

type PrintRequest struct {
	Token     string `json:"token"`
	Title     string `json:"title"`
	OrderID   string `json:"order_id"`
	Body      string `json:"body"`
	QRValue   string `json:"qr_value"`
	PrintMode string `json:"print_mode"` // "receipt-only", "qr-only", or empty (default: both)
}

// ================= MAIN =================

func main() {
	http.HandleFunc("/ping", corsMiddleware(pingHandler))
	http.HandleFunc("/print", corsMiddleware(printHandler))

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

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","agent":"cleanlink-printer"}`))
}

func printHandler(w http.ResponseWriter, r *http.Request) {
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

	receipt := []byte{}

	// Initialize printer
	receipt = append(receipt, escInit()...)

	// Determine print mode
	printMode := req.PrintMode
	if printMode == "" {
		// Default: if QR value exists, print both; otherwise receipt only
		if req.QRValue != "" {
			printMode = "all"
		} else {
			printMode = "receipt-only"
		}
	}

	// Top spacing
	receipt = append(receipt, []byte("\n")...)

	// SEPARATOR MODE: Print barrier between customer receipt and staff labels
	if printMode == "separator" {
		// Add spacing before separator
		receipt = append(receipt, []byte("\n\n")...)

		// Separator line
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)

		// Text: --- Untuk Staff ---
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("--- Untuk Staff ---\n")...)
		receipt = append(receipt, escBold(false)...)

		// Separator line
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)

		// Bottom spacing
		receipt = append(receipt, []byte("\n\n")...)

		// Paper cut
		receipt = append(receipt, escCut()...)
	} else if printMode == "qr-only" {
		// QR-ONLY MODE: Print only QR code label (for internal tracking)
		// Staff label at the top
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, escBold(false)...)

		// Header - Service name
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

		// Body content (customer info, deadline, etc.) - Left aligned
		receipt = append(receipt, escAlignLeft()...)
		receipt = append(receipt, []byte(req.Body)...)

		// Separator before QR code
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("\n--------------------------------\n")...)

		// QR CODE - Centered with larger size for better scanning
		receipt = append(receipt, []byte("\n")...)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escQRCode(req.QRValue)...)

		// Text below QR code
		receipt = append(receipt, []byte("\n\n")...)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("Scan untuk update status\n")...)
		receipt = append(receipt, escBold(false)...)

		// Bottom spacing before cut
		receipt = append(receipt, []byte("\n\n")...)
		receipt = append(receipt, escCut()...)
	} else {
		// RECEIPT-ONLY MODE or ALL MODE: Print receipt
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

		// QR code on receipt (only if print mode is "all" and QR value exists)
		// Note: In "print all" mode, QR codes are printed separately as labels
		// So we don't print QR code here in receipt mode

		// Bottom spacing and thank you message
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("\n================================\n")...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte("Terima kasih\n")...)
		receipt = append(receipt, escBold(false)...)
		receipt = append(receipt, []byte("Atas kepercayaan Anda\n")...)

		// Bottom spacing before cut
		receipt = append(receipt, []byte("\n\n")...)

		// Paper cut
		receipt = append(receipt, escCut()...)
	}

	err := os.WriteFile(PRINTER_DEV, receipt, 0666)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"printed"}`))
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
