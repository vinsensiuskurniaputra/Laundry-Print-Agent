//go:build console
// +build console

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	PORT      = ":3491"
	API_TOKEN = "CLEANLINK_SECRET_123"
)

// Global variable to store selected printer COM port
var selectedPrinterCOM string
var serverRunning bool

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

// PrinterInfo holds printer information
type PrinterInfo struct {
	Name     string
	DeviceID string
}

// ================= MAIN =================

func main() {
	fmt.Println("========================================")
	fmt.Println("   Cleanlink Printer Agent - Console")
	fmt.Println("========================================")
	fmt.Println()

	// Step 1: Detect printers
	fmt.Println("🔍 Detecting printers...")
	fmt.Println("   Scanning Bluetooth and USB devices...")
	printers, err := detectAllPrinters()
	if err != nil || len(printers) == 0 {
		fmt.Println("❌ No printers found!")
		fmt.Println("   Please check if your printer is connected.")
		fmt.Println()
		fmt.Print("Press Enter to exit...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		return
	}

	// Step 2: Show available printers
	fmt.Println()
	fmt.Println("📋 Available Printers:")
	fmt.Println("========================================")
	for i, printer := range printers {
		// Determine device type icon
		deviceType := "🔌"
		if strings.Contains(strings.ToLower(printer.Name), "bluetooth") {
			deviceType = "�"
		} else if strings.Contains(strings.ToLower(printer.Name), "usb") {
			deviceType = "🔌"
		} else if strings.Contains(strings.ToLower(printer.Name), "serial") {
			deviceType = "🔗"
		}

		fmt.Printf("\n[%d] %s %s\n", i+1, deviceType, printer.Name)
		fmt.Printf("    📍 Port: %s\n", printer.DeviceID)
	}
	fmt.Println("\n========================================")

	// Step 3: Let user select printer
	reader := bufio.NewReader(os.Stdin)
	var selectedIndex int
	for {
		fmt.Print("\n👉 Select printer number (1-", len(printers), "): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		index, err := strconv.Atoi(input)
		if err == nil && index >= 1 && index <= len(printers) {
			selectedIndex = index - 1
			break
		}
		fmt.Println("❌ Invalid selection. Please try again.")
	}

	selectedPrinter := printers[selectedIndex]
	selectedPrinterCOM = selectedPrinter.DeviceID

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("✅ Selected Printer")
	fmt.Println("========================================")
	fmt.Println("🖨️  Device Name:", selectedPrinter.Name)
	fmt.Println("📍 COM Port:", selectedPrinterCOM)
	fmt.Println("========================================")

	// Step 4: Test printer connection
	fmt.Println()
	fmt.Print("🧪 Would you like to test the printer? (y/n): ")
	testInput, _ := reader.ReadString('\n')
	testInput = strings.TrimSpace(strings.ToLower(testInput))

	if testInput == "y" || testInput == "yes" {
		fmt.Println("📄 Sending test print...")
		testPrint()
	}

	// Step 5: Start server
	fmt.Println()
	fmt.Println("🚀 Starting HTTP server...")

	http.HandleFunc("/ping", corsMiddleware(ping))
	http.HandleFunc("/print", corsMiddleware(printHandler))
	http.HandleFunc("/check", corsMiddleware(checkPrinter))

	serverRunning = true

	fmt.Println()
	fmt.Println("✅ Server is running!")
	fmt.Println("========================================")
	fmt.Println("📡 Endpoint: http://localhost" + PORT)
	fmt.Println("🖨️  Printer: " + selectedPrinterCOM)
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the server")
	fmt.Println()

	if err := http.ListenAndServe(PORT, nil); err != nil {
		fmt.Println("❌ Server error:", err)
	}
}

func testPrint() {
	receipt := []byte{}
	receipt = append(receipt, escInit()...)
	receipt = append(receipt, []byte("\n")...)
	receipt = append(receipt, escAlignCenter()...)
	receipt = append(receipt, escBold(true)...)
	receipt = append(receipt, escDoubleHeight(true)...)
	receipt = append(receipt, []byte("TEST PRINT\n")...)
	receipt = append(receipt, escDoubleHeight(false)...)
	receipt = append(receipt, escBold(false)...)
	receipt = append(receipt, escAlignCenter()...)
	receipt = append(receipt, []byte("================================\n")...)
	receipt = append(receipt, escAlignLeft()...)
	receipt = append(receipt, []byte("This is a test print.\n")...)
	receipt = append(receipt, []byte("Printer is working correctly!\n")...)
	receipt = append(receipt, []byte("\n")...)
	receipt = append(receipt, escAlignCenter()...)
	receipt = append(receipt, []byte("Cleanlink Printer Agent\n")...)
	receipt = append(receipt, []byte("\n")...)
	receipt = append(receipt, escCut()...)

	err := writeToCOM(selectedPrinterCOM, receipt)
	if err != nil {
		fmt.Println("❌ Test print failed:", err)
	} else {
		fmt.Println("✅ Test print completed successfully!")
	}
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

	// Use the globally selected printer COM port
	com := selectedPrinterCOM
	if com == "" {
		w.Write([]byte(`{"status":"error","message":"No printer selected","com":""}`))
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

	// Use the globally selected printer COM port
	com := selectedPrinterCOM
	if com == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "No printer selected", 500)
		return
	}

	fmt.Printf("[DEBUG] Using printer on COM port: %s for Order ID: %s\n", com, req.OrderID)

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

	err := writeToCOM(com, receipt)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Printf("[DEBUG] Successfully printed Order ID: %s to %s\n", req.OrderID, com)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"printed","com":"` + com + `"}`))
}

// ================= CORE =================

// detectAllPrinters returns all available serial printers (Bluetooth and USB)
func detectAllPrinters() ([]PrinterInfo, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("Windows only")
	}

	var allPrinters []PrinterInfo

	// Create context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Query all serial ports (not just Bluetooth)
	cmd := exec.CommandContext(
		ctx,
		"powershell",
		"-NoProfile",
		"-Command",
		`Get-CimInstance Win32_SerialPort | Select-Object Name, DeviceID | ConvertTo-Json`,
	)

	out, err := cmd.Output()
	if err == nil {
		output := strings.TrimSpace(string(out))
		if output != "" {
			var printers []PrinterInfo

			// Handle both single object and array of objects
			if strings.HasPrefix(output, "[") {
				err = json.Unmarshal([]byte(output), &printers)
			} else {
				var single PrinterInfo
				err = json.Unmarshal([]byte(output), &single)
				if err == nil {
					printers = append(printers, single)
				}
			}

			if err == nil && len(printers) > 0 {
				allPrinters = append(allPrinters, printers...)
			}
		}
	}

	// Also try alternative detection for common COM ports
	commonPorts := []string{"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM10", "COM11", "COM12"}
	for _, port := range commonPorts {
		testPort := `\\.\` + port
		f, err := os.OpenFile(testPort, os.O_WRONLY, 0)
		if err == nil {
			f.Close()
			// Check if this port is not already in the list
			found := false
			for _, p := range allPrinters {
				if p.DeviceID == port {
					found = true
					break
				}
			}
			if !found {
				allPrinters = append(allPrinters, PrinterInfo{
					Name:     fmt.Sprintf("Serial Printer on %s", port),
					DeviceID: port,
				})
			}
		}
	}

	if len(allPrinters) == 0 {
		return nil, errors.New("No printers found")
	}

	return allPrinters, nil
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
