package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
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

// ================= MAIN =================

func main() {
	// Create GUI application
	myApp := app.NewWithID("com.cleanlink.printer")
	myWindow := myApp.NewWindow("Cleanlink Printer Agent")
	myWindow.Resize(fyne.NewSize(500, 450))

	// Status label
	statusLabel := widget.NewLabel("Status: Not Started")
	statusLabel.Alignment = fyne.TextAlignCenter

	// Server info label
	serverInfoLabel := widget.NewLabel("")
	serverInfoLabel.Alignment = fyne.TextAlignCenter
	serverInfoLabel.Wrapping = fyne.TextWrapWord

	// Printer selection
	printerLabel := widget.NewLabel("Selected Printer: None")
	printerLabel.Alignment = fyne.TextAlignCenter

	// Debug label to show what's happening
	debugLabel := widget.NewLabel("Debug: Ready")
	debugLabel.Alignment = fyne.TextAlignCenter
	debugLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// Detect available printers button
	var printerSelect *widget.Select
	var detectButton *widget.Button
	detectButton = widget.NewButton("Detect Printers", func() {
		// Disable button during detection
		detectButton.Disable()
		statusLabel.SetText("Status: Detecting printers...")

		// Create progress dialog
		progressBar := widget.NewProgressBarInfinite()
		progressDialog := dialog.NewCustom(
			"Detecting Printers",
			"",
			container.NewVBox(
				progressBar,
				widget.NewLabel("Scanning for Bluetooth and USB printers..."),
				widget.NewLabel("This may take a few seconds..."),
			),
			myWindow,
		)
		progressDialog.Show()

		// Run detection in background
		go func() {
			printers, err := detectAllPrinters()

			// Close progress dialog and enable button after detection
			progressDialog.Hide()
			detectButton.Enable()

			if err != nil || len(printers) == 0 {
				dialog.ShowError(errors.New("No printers found. Please check connections."), myWindow)
				statusLabel.SetText("Status: No printers found")
				statusLabel.Refresh()
				return
			}

			// Create options for select widget
			options := make([]string, len(printers))
			for i, p := range printers {
				options[i] = fmt.Sprintf("%s (%s)", p.Name, p.DeviceID)
			}

			// Extract COM port from first option for auto-select
			var firstCOMPort string
			if len(options) > 0 {
				firstOption := options[0]
				// Find the last occurrence of "(" to get the COM port
				lastParenIndex := strings.LastIndex(firstOption, "(")
				if lastParenIndex != -1 {
					firstCOMPort = strings.TrimSuffix(firstOption[lastParenIndex+1:], ")")
				}
			}

			// Update the select widget options
			printerSelect.Options = options
			printerSelect.Refresh()

			// Auto-select first printer - Set global variable FIRST before setting dropdown
			if len(options) > 0 && firstCOMPort != "" {
				selectedPrinterCOM = firstCOMPort
				fmt.Printf("[DEBUG] Auto-selected printer: %s\n", firstCOMPort)
				debugLabel.SetText(fmt.Sprintf("Debug: Auto-select %s", firstCOMPort))
				debugLabel.Refresh()

				// Update label directly
				printerLabel.SetText(fmt.Sprintf("Selected Printer: %s", firstCOMPort))
				printerLabel.Refresh()

				// Set the selected value in dropdown (this will trigger callback but variable is already set)
				printerSelect.SetSelected(options[0])
			}

			statusLabel.SetText(fmt.Sprintf("Status: Found %d printer(s)", len(printers)))
			statusLabel.Refresh()
		}()
	})

	// Printer select dropdown
	printerSelect = widget.NewSelect([]string{}, func(value string) {
		// Extract COM port from selection (format: "Name (COMx)")
		fmt.Printf("[DEBUG] Printer selection changed to: %s\n", value)

		if value != "" {
			// Find the last occurrence of "(" to get the COM port
			// This handles cases like "Standard Serial over Bluetooth link (COM10) (COM10)"
			lastParenIndex := strings.LastIndex(value, "(")
			if lastParenIndex != -1 {
				comPort := strings.TrimSuffix(value[lastParenIndex+1:], ")")
				selectedPrinterCOM = comPort
				fmt.Printf("[DEBUG] COM port set to: %s\n", comPort)

				// Update both labels
				debugLabel.SetText(fmt.Sprintf("Debug: Selected %s", comPort))
				printerLabel.SetText(fmt.Sprintf("Selected Printer: %s", comPort))

				// Force complete UI refresh
				debugLabel.Refresh()
				printerLabel.Refresh()
			} else {
				fmt.Printf("[DEBUG] Failed to parse COM port from: %s\n", value)
			}
		}
	})
	printerSelect.PlaceHolder = "Select a printer..."

	// Start server button
	startButton := widget.NewButton("Start Server", func() {
		fmt.Printf("[DEBUG] Start server clicked. Selected COM port: '%s'\n", selectedPrinterCOM)
		if selectedPrinterCOM == "" {
			dialog.ShowError(errors.New("Please select a printer first"), myWindow)
			return
		}

		if serverRunning {
			dialog.ShowInformation("Info", "Server is already running", myWindow)
			return
		}

		// Start HTTP server in goroutine
		go func() {
			http.HandleFunc("/ping", corsMiddleware(ping))
			http.HandleFunc("/print", corsMiddleware(printHandler))
			http.HandleFunc("/check", corsMiddleware(checkPrinter))

			serverRunning = true
			statusLabel.SetText("Status: Server Running ✓")
			serverInfoLabel.SetText(fmt.Sprintf("Server: http://localhost%s\nPrinter: %s\nReady to accept print jobs", PORT, selectedPrinterCOM))

			fmt.Println("Cleanlink Printer Agent running on", PORT)
			fmt.Println("Using printer:", selectedPrinterCOM)

			if err := http.ListenAndServe(PORT, nil); err != nil {
				fmt.Println("Server error:", err)
				serverRunning = false
				statusLabel.SetText("Status: Server Error")
			}
		}()

		time.Sleep(500 * time.Millisecond) // Give server time to start
		dialog.ShowInformation("Success", fmt.Sprintf("Server started successfully!\n\nEndpoint: http://localhost%s\nPrinter: %s", PORT, selectedPrinterCOM), myWindow)
	})
	startButton.Importance = widget.HighImportance

	// Test print button
	testButton := widget.NewButton("Test Print", func() {
		if !serverRunning {
			dialog.ShowError(errors.New("Please start the server first"), myWindow)
			return
		}

		go func() {
			// Create a sample receipt body following the format from the image
			sampleBody := "Nama       : Bu Kayam\n"
			sampleBody += "Alamat     : Villa Nusa Indah 2 blok 5. No. 29\n"
			sampleBody += "Telp       : 0812 934 823\n"
			sampleBody += "--------------------------------\n"
			sampleBody += "Nomor      : VLN2 000 000 01\n"
			sampleBody += "Layanan    : Cuci Setrika\n"
			sampleBody += "Berat      : 13 kg\n"
			sampleBody += "Jumlah     :\n"
			sampleBody += "Hrg satuan : Rp. 8.000\n"
			sampleBody += "Sub Total  : Rp. 104.000\n"
			sampleBody += "\n"
			sampleBody += "Pembayaran : Cash\n"
			sampleBody += "Bayar      : Rp. 105.000\n"
			sampleBody += "Kembali    : Rp. 1.000\n"
			sampleBody += "\n"
			sampleBody += "Masuk      : 12/22/2025, Jam : 16.45\n"
			sampleBody += "+- Selesai : 12/23/2025, Jam : 16.45\n"
			sampleBody += "--------------------------------\n"

			// Send test print request
			testData := PrintRequest{
				Token:     API_TOKEN,
				Title:     "Smart Laundry Test",
				OrderID:   "TEST-001",
				Body:      sampleBody,
				PrintMode: "receipt-only",
			}

			receipt := []byte{}
			receipt = append(receipt, escInit()...)
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte(testData.Title+"\n")...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignLeft()...)
			receipt = append(receipt, escFontSize(false)...) // Use small font
			receipt = append(receipt, []byte(testData.Body)...)
			receipt = append(receipt, escFontSize(true)...) // Back to normal font
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("Terimakasih\n")...)
			receipt = append(receipt, []byte("Atas Kepercayaan Anda\n")...)
			receipt = append(receipt, []byte("\n\n\n\n\n\n")...)
			receipt = append(receipt, escCut()...)
			receipt = append(receipt, escInit()...)

			err := writeToCOM(selectedPrinterCOM, receipt)
			if err != nil {
				dialog.ShowError(fmt.Errorf("Print failed: %v", err), myWindow)
			} else {
				dialog.ShowInformation("Success", "Test print completed!", myWindow)
			}
		}()
	})

	// Title
	title := canvas.NewText("Cleanlink Printer Agent", color.RGBA{R: 0, G: 122, B: 204, A: 255})
	title.TextSize = 24
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}

	// Instructions
	instructions := widget.NewLabel("1. Connect your printer via Bluetooth or USB\n2. Click 'Detect Printers' to scan for available printers\n3. Select your printer from the dropdown\n4. Click 'Start Server' to begin accepting print jobs")
	instructions.Wrapping = fyne.TextWrapWord

	// Layout
	content := container.NewVBox(
		layout.NewSpacer(),
		title,
		layout.NewSpacer(),
		instructions,
		widget.NewSeparator(),
		detectButton,
		printerSelect,
		printerLabel,
		debugLabel,
		layout.NewSpacer(),
		startButton,
		testButton,
		layout.NewSpacer(),
		widget.NewSeparator(),
		statusLabel,
		serverInfoLabel,
		layout.NewSpacer(),
	)

	myWindow.SetContent(content)
	myWindow.CenterOnScreen()

	// Show and run
	myWindow.ShowAndRun()
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

		// Header - Branch name (Centered, Bold)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, escDoubleHeight(true)...)
		receipt = append(receipt, []byte(req.Title+"\n")...)
		receipt = append(receipt, escDoubleHeight(false)...)
		receipt = append(receipt, escBold(false)...)

		// Separator under Title
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)

		// Spacing after title
		receipt = append(receipt, []byte("\n")...)

		// Body content - Left aligned
		receipt = append(receipt, escAlignLeft()...)
		receipt = append(receipt, []byte(req.Body)...)

		// Bottom spacing
		receipt = append(receipt, []byte("\n")...)

		// Thank you message - Centered
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("Terimakasih\n")...)
		receipt = append(receipt, []byte("Atas Kepercayaan Anda\n")...)

		// Bottom spacing before cut
		receipt = append(receipt, []byte("\n\n\n\n\n\n")...)
		receipt = append(receipt, escCut()...)
		receipt = append(receipt, escInit()...)
	} else if printMode == "qr-only" {
		// QR-ONLY MODE: Print only QR code labels
		// Print all QR codes from the array
		for i, qrData := range req.QRCodes {
			if i > 0 {
				// Add spacing between QR codes
				receipt = append(receipt, []byte("\n\n\n\n")...)
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
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escQRCode(qrData.QRValue)...)
			receipt = append(receipt, []byte("\n")...)

			// Text below QR code
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("Scan untuk update status\n")...)
			receipt = append(receipt, escBold(false)...)

			// Bottom spacing before cut (only after last QR code)
			if i == len(req.QRCodes)-1 {
				receipt = append(receipt, []byte("\n\n\n\n\n\n")...)
				receipt = append(receipt, escCut()...)
				receipt = append(receipt, escInit()...)
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
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escQRCode(req.QRValue)...)
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("Scan untuk update status\n")...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, []byte("\n\n\n\n\n\n")...)
			receipt = append(receipt, escCut()...)
			receipt = append(receipt, escInit()...)
		}
	} else if printMode == "label" {
		// LABEL MODE: Print detailed label (Branch Name, Details, QR)
		// Print all QR codes from the array
		for i, qrData := range req.QRCodes {
			if i > 0 {
				// Add spacing between QR codes
				receipt = append(receipt, []byte("\n\n\n\n")...)
			}

			// 1. Branch Name (Title) - Centered, Bold
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, escDoubleHeight(true)...)
			receipt = append(receipt, []byte(req.Title+"\n")...)
			receipt = append(receipt, escDoubleHeight(false)...)
			receipt = append(receipt, escBold(false)...)

			// 2. Separator under Title
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)

			// 3. Body (Details) - Left aligned
			receipt = append(receipt, escAlignLeft()...)
			receipt = append(receipt, []byte(qrData.Body)...)

			// 4. Separator before QR
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)

			// 5. Scan Barcode Text
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("Scan Barcode\n")...)

			// 6. QR Code
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escQRCode(qrData.QRValue)...)
			receipt = append(receipt, []byte("\n")...)

			// Cut paper only after last QR code
			if i == len(req.QRCodes)-1 {
				receipt = append(receipt, []byte("\n\n\n\n\n\n")...)
				receipt = append(receipt, escCut()...)
				receipt = append(receipt, escInit()...)
			}
		}
	} else {
		// ALL MODE: Print receipt, separator, then all QR codes
		// 1. Print receipt
		receipt = append(receipt, []byte("\n")...)

		// Header - Branch name (Centered, Bold)
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, escBold(true)...)
		receipt = append(receipt, []byte(req.Title+"\n")...)
		receipt = append(receipt, escBold(false)...)

		// Separator under Title
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("--------------------------------\n")...)

		// Spacing after title
		receipt = append(receipt, []byte("\n")...)

		// Body content - Left aligned
		receipt = append(receipt, escAlignLeft()...)
		receipt = append(receipt, []byte(req.Body)...)

		// Bottom spacing
		receipt = append(receipt, []byte("\n")...)

		// Thank you message - Centered
		receipt = append(receipt, escAlignCenter()...)
		receipt = append(receipt, []byte("Terimakasih\n")...)
		receipt = append(receipt, []byte("Atas Kepercayaan Anda\n")...)

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
				receipt = append(receipt, []byte("\n\n\n\n")...)
			}

			// 1. Branch Name (Title) - Centered, Bold
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, escDoubleHeight(true)...)
			receipt = append(receipt, []byte(req.Title+"\n")...)
			receipt = append(receipt, escDoubleHeight(false)...)
			receipt = append(receipt, escBold(false)...)

			// 2. Separator under Title
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)

			// 3. Body (Details) - Left aligned
			receipt = append(receipt, escAlignLeft()...)
			receipt = append(receipt, []byte(qrData.Body)...)

			// 4. Separator before QR
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("--------------------------------\n")...)

			// 5. Scan Barcode Text
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, []byte("Scan Barcode\n")...)

			// 6. QR Code
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, escQRCode(qrData.QRValue)...)
			receipt = append(receipt, []byte("\n")...)

			// Cut paper only after last QR code
			if i == len(req.QRCodes)-1 {
				receipt = append(receipt, []byte("\n\n\n\n\n\n")...)
				receipt = append(receipt, escCut()...)
				receipt = append(receipt, escInit()...)
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
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escQRCode(req.QRValue)...)
			receipt = append(receipt, []byte("\n")...)
			receipt = append(receipt, []byte("\n\n")...)
			receipt = append(receipt, escAlignCenter()...)
			receipt = append(receipt, escBold(true)...)
			receipt = append(receipt, []byte("Scan untuk update status\n")...)
			receipt = append(receipt, escBold(false)...)
			receipt = append(receipt, []byte("\n\n\n\n\n\n")...)
			receipt = append(receipt, escCut()...)
			receipt = append(receipt, escInit()...)
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

func detectPrinterCOM() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("Windows only")
	}

	// Create context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use Get-CimInstance to get both printer name and COM port
	cmd := exec.CommandContext(
		ctx,
		"powershell",
		"-NoProfile",
		"-Command",
		`Get-CimInstance Win32_SerialPort | Where-Object { $_.Name -like "*Bluetooth*" } | Select-Object Name, DeviceID | ConvertTo-Json`,
	)

	out, err := cmd.Output()
	if err != nil {
		fmt.Println("[DEBUG] PowerShell command failed, using alternative detection")
		// If command fails, try alternative method: check common COM ports
		return detectPrinterCOMAlternative()
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		fmt.Println("[DEBUG] No Bluetooth printer found via PowerShell, using alternative detection")
		return detectPrinterCOMAlternative()
	}

	// Parse JSON output
	var printers []struct {
		Name     string
		DeviceID string
	}

	// Handle both single object and array of objects
	if strings.HasPrefix(output, "[") {
		err = json.Unmarshal([]byte(output), &printers)
	} else {
		var single struct {
			Name     string
			DeviceID string
		}
		err = json.Unmarshal([]byte(output), &single)
		if err == nil {
			printers = append(printers, single)
		}
	}

	if err != nil || len(printers) == 0 {
		fmt.Println("[DEBUG] Failed to parse printer info, using alternative detection")
		return detectPrinterCOMAlternative()
	}

	// Use the first Bluetooth printer found
	printer := printers[0]
	fmt.Printf("[DEBUG] Connected to printer: %s on port %s\n", printer.Name, printer.DeviceID)

	return printer.DeviceID, nil
}

// PrinterInfo holds printer information
type PrinterInfo struct {
	Name     string
	DeviceID string
}

// detectAllPrinters returns all available serial printers (Bluetooth and USB)
func detectAllPrinters() ([]PrinterInfo, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("Windows only")
	}

	var allPrinters []PrinterInfo

	// Create context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Query all serial ports (Bluetooth and USB serial adapters)
	fmt.Println("[DEBUG] Detecting serial ports (Bluetooth and USB)...")
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
				fmt.Printf("[DEBUG] Found %d serial port(s)\n", len(printers))
				allPrinters = append(allPrinters, printers...)
			}
		}
	}

	// 2. Query USB printers (POS printers with USB interface)
	fmt.Println("[DEBUG] Detecting USB printers...")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	// Query USB devices that are printers or POS devices
	usbCmd := exec.CommandContext(
		ctx2,
		"powershell",
		"-NoProfile",
		"-Command",
		`Get-CimInstance Win32_USBControllerDevice | ForEach-Object { [wmi]($_.Dependent) } | Where-Object { $_.Description -like "*printer*" -or $_.Description -like "*POS*" -or $_.Description -like "*USB*" } | Select-Object @{Name='Name';Expression={$_.Description}}, @{Name='DeviceID';Expression={$_.DeviceID}} | ConvertTo-Json`,
	)

	usbOut, usbErr := usbCmd.Output()
	if usbErr == nil {
		usbOutput := strings.TrimSpace(string(usbOut))
		if usbOutput != "" {
			var usbDevices []PrinterInfo

			// Handle both single object and array of objects
			if strings.HasPrefix(usbOutput, "[") {
				err = json.Unmarshal([]byte(usbOutput), &usbDevices)
			} else {
				var single PrinterInfo
				err = json.Unmarshal([]byte(usbOutput), &single)
				if err == nil {
					usbDevices = append(usbDevices, single)
				}
			}

			if err == nil && len(usbDevices) > 0 {
				fmt.Printf("[DEBUG] Found %d USB printer device(s)\n", len(usbDevices))
				// Add USB devices, but check for duplicates
				for _, usb := range usbDevices {
					found := false
					for _, existing := range allPrinters {
						if existing.DeviceID == usb.DeviceID {
							found = true
							break
						}
					}
					if !found {
						allPrinters = append(allPrinters, usb)
					}
				}
			}
		}
	}

	// 3. Also try alternative detection for common COM ports (fallback method)
	fmt.Println("[DEBUG] Checking common COM ports...")
	commonPorts := []string{"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM10", "COM11", "COM12", "COM13", "COM14", "COM15", "COM16"}
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
					Name:     fmt.Sprintf("Serial/USB Printer on %s", port),
					DeviceID: port,
				})
				fmt.Printf("[DEBUG] Found accessible port: %s\n", port)
			}
		}
	}

	if len(allPrinters) == 0 {
		return nil, errors.New("No printers found. Please ensure your printer is connected via Bluetooth or USB")
	}

	fmt.Printf("[DEBUG] Total printers detected: %d\n", len(allPrinters))
	return allPrinters, nil
}

func detectPrinterCOMAlternative() (string, error) {
	// Try common COM ports for Bluetooth printers
	// Based on the image, the user's printer is on COM10, so check it first
	commonPorts := []string{"COM10", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM11", "COM12"}

	fmt.Println("[DEBUG] Trying alternative detection method...")
	for _, port := range commonPorts {
		testPort := `\\.\` + port
		f, err := os.OpenFile(testPort, os.O_WRONLY, 0)
		if err == nil {
			f.Close()
			fmt.Printf("[DEBUG] Found accessible printer on port: %s\n", port)
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

	// Size (6) - Optimized for 57mm paper (good balance between size and scannability)
	// Size range: 1-16
	// For 57mm paper: 6-8 is recommended, 6 is optimal for higher density and scannability
	// If QR code is too small, increase to 7 or 8.
	cmd = append(cmd, []byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x43, 0x06}...)

	// Error correction level (H - High) for maximum reliability
	// 0x30 = L (Low), 0x31 = M (Medium), 0x32 = Q (Quartile), 0x33 = H (High)
	cmd = append(cmd, []byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x45, 0x33}...)

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
