package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type STMDocument struct {
	Name       string
	URL        string
	ChipFamily string
	DocType    string
}

// Curated list of essential STM32 documents
var stm32Docs = []STMDocument{
	// STM32F4 Family
	{
		Name:       "STM32F4_Reference_Manual_RM0090.pdf",
		URL:        "https://www.st.com/resource/en/reference_manual/rm0090-stm32f405415-stm32f407417-stm32f427437-and-stm32f429439-advanced-armbased-32bit-mcus-stmicroelectronics.pdf",
		ChipFamily: "STM32F4",
		DocType:    "reference_manual",
	},
	{
		Name:       "STM32F407_Datasheet.pdf",
		URL:        "https://www.st.com/resource/en/datasheet/stm32f407vg.pdf",
		ChipFamily: "STM32F4",
		DocType:    "datasheet",
	},
	{
		Name:       "STM32F4_Programming_Manual_PM0214.pdf",
		URL:        "https://www.st.com/resource/en/programming_manual/pm0214-stm32-cortexm4-mcus-and-mpus-programming-manual-stmicroelectronics.pdf",
		ChipFamily: "STM32F4",
		DocType:    "programming_manual",
	},

	// STM32F7 Family
	{
		Name:       "STM32F7_Reference_Manual_RM0385.pdf",
		URL:        "https://www.st.com/resource/en/reference_manual/rm0385-stm32f75xxx-and-stm32f74xxx-advanced-armbased-32bit-mcus-stmicroelectronics.pdf",
		ChipFamily: "STM32F7",
		DocType:    "reference_manual",
	},

	// STM32H7 Family
	{
		Name:       "STM32H7_Reference_Manual_RM0433.pdf",
		URL:        "https://www.st.com/resource/en/reference_manual/rm0433-stm32h742-stm32h743753-and-stm32h750-value-line-advanced-armbased-32bit-mcus-stmicroelectronics.pdf",
		ChipFamily: "STM32H7",
		DocType:    "reference_manual",
	},

	// Add more as needed...
}

func main() {
	outputDir := "testdata/stm_docs"

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Failed to create output directory: %v\n", err)
		return
	}

	fmt.Printf("📥 Downloading %d STM32 documents...\n\n", len(stm32Docs))

	successCount := 0
	failCount := 0

	for i, doc := range stm32Docs {
		fmt.Printf("[%d/%d] %s\n", i+1, len(stm32Docs), doc.Name)
		fmt.Printf("    Family: %s, Type: %s\n", doc.ChipFamily, doc.DocType)

		outputPath := filepath.Join(outputDir, doc.Name)

		// Check if already downloaded
		if _, err := os.Stat(outputPath); err == nil {
			fmt.Printf("    ✓ Already exists, skipping\n\n")
			successCount++
			continue
		}

		// Download
		err := downloadFile(doc.URL, outputPath)
		if err != nil {
			fmt.Printf("    ✗ Failed: %v\n\n", err)
			failCount++
			continue
		}

		// Get file size
		fileInfo, _ := os.Stat(outputPath)
		sizeMB := float64(fileInfo.Size()) / 1024 / 1024

		fmt.Printf("    ✓ Downloaded (%.2f MB)\n\n", sizeMB)
		successCount++

		// Be nice to ST's servers
		time.Sleep(2 * time.Second)
	}

	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("   ✓ Success: %d\n", successCount)
	fmt.Printf("   ✗ Failed: %d\n", failCount)
	fmt.Printf("   📁 Location: %s\n", outputDir)
}

func downloadFile(url, filepath string) error {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	// Make request
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create file
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer out.Close()

	// Copy data
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	return nil
}
