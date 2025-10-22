package instance

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/WhyIsSandwich/factctl/internal/auth"
	"github.com/ulikunitz/xz"
)

// FactorioDownloader handles downloading Factorio releases
type FactorioDownloader struct {
	baseDir string
}

// NewFactorioDownloader creates a new Factorio downloader
func NewFactorioDownloader(baseDir string) *FactorioDownloader {
	return &FactorioDownloader{
		baseDir: baseDir,
	}
}

// DownloadFactorio downloads and installs a specific Factorio version
func (fd *FactorioDownloader) DownloadFactorio(ctx context.Context, version string) (string, error) {
	return fd.DownloadFactorioWithBuild(ctx, version, "alpha")
}

// DownloadFactorioWithBuild downloads and installs a specific Factorio version with a specific build type
func (fd *FactorioDownloader) DownloadFactorioWithBuild(ctx context.Context, version, buildType string) (string, error) {
	return fd.DownloadFactorioWithName(ctx, version, buildType, "")
}

// DownloadFactorioWithName downloads and installs a specific Factorio version with a specific build type and optional name
func (fd *FactorioDownloader) DownloadFactorioWithName(ctx context.Context, version, buildType, name string) (string, error) {
	// Get credentials
	creds, err := fd.getCredentials()
	if err != nil {
		return "", fmt.Errorf("getting credentials: %w", err)
	}

	// Determine the appropriate distribution for the current platform
	distro, err := fd.getDistribution(buildType)
	if err != nil {
		return "", fmt.Errorf("determining distribution: %w", err)
	}

	// Download URL
	downloadURL := fmt.Sprintf("https://www.factorio.com/get-download/%s/%s/%s?username=%s&token=%s",
		version, buildType, distro, creds.FactorioUsername, creds.FactorioToken)

	fmt.Printf("Downloading Factorio %s for %s...\n", version, distro)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	// Download the file
	client := &http.Client{
		Timeout: 30 * time.Minute, // Factorio downloads can be large
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading Factorio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Generate smart runtime name if not provided
	if name == "" {
		name = fd.generateRuntimeName(version, buildType)
	}

	// Create runtime directory
	runtimeDir := filepath.Join(fd.baseDir, "runtimes")
	versionDir := filepath.Join(runtimeDir, name)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return "", fmt.Errorf("creating version directory: %w", err)
	}

	// Extract the downloaded archive
	if err := fd.extractArchive(resp.Body, versionDir, distro); err != nil {
		return "", fmt.Errorf("extracting archive: %w", err)
	}

	fmt.Printf("Factorio %s installed to %s\n", version, versionDir)
	return versionDir, nil
}

// getCredentials retrieves Factorio credentials
func (fd *FactorioDownloader) getCredentials() (*auth.Credentials, error) {
	// Try to get config directory from base directory
	configDir := filepath.Join(fd.baseDir, "config")
	store := auth.NewStore(configDir)

	creds, err := store.Load()
	if err != nil {
		// Try default location as fallback
		if defaultPath, err := auth.DefaultLocation(); err == nil {
			defaultStore := auth.NewStore(filepath.Dir(defaultPath))
			creds, err = defaultStore.Load()
		}
	}

	if err != nil || creds.FactorioUsername == "" || creds.FactorioToken == "" {
		return nil, fmt.Errorf("Factorio credentials not found\nHint: Run 'factctl auth' to authenticate with your Factorio account")
	}

	return creds, nil
}

// getDistribution determines the appropriate distribution for the current platform and build type
func (fd *FactorioDownloader) getDistribution(buildType string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		if buildType == "headless" {
			return "win64-manual", nil
		}
		return "win64-manual", nil
	case "darwin":
		if buildType == "headless" {
			return "osx", nil
		}
		return "osx", nil
	case "linux":
		if buildType == "headless" {
			return "linux64", nil
		}
		return "linux64", nil
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// extractArchive extracts the downloaded Factorio archive
func (fd *FactorioDownloader) extractArchive(reader io.Reader, destDir string, distro string) error {
	// Handle different archive types based on distribution
	if strings.Contains(distro, "win64") {
		// Windows ZIP file
		return fd.extractZIP(reader, destDir)
	} else if strings.Contains(distro, "osx") {
		// macOS DMG file - extract the Factorio.app bundle
		return fd.extractDMG(reader, destDir)
	} else if strings.Contains(distro, "linux64") {
		// Linux file - could be tar.gz, 7z, or other formats
		return fd.extractLinuxArchive(reader, destDir)
	}

	return fmt.Errorf("unsupported distribution: %s", distro)
}

// extractLinuxArchive detects and extracts Linux archives (tar.gz, 7z, xz, etc.)
func (fd *FactorioDownloader) extractLinuxArchive(reader io.Reader, destDir string) error {
	// Read all data first to detect format
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("reading archive data: %w", err)
	}

	// Check magic bytes to determine format and create appropriate stream
	var stream io.Reader
	var streamType string

	if len(data) >= 6 {
		// Check for XZ/7z format (magic bytes: 0xFD 0x37 0x7A 0x58 0x5A 0x00)
		if data[0] == 0xFD && data[1] == 0x37 && data[2] == 0x7A && data[3] == 0x58 && data[4] == 0x5A && data[5] == 0x00 {
			stream, err = fd.createXZStream(bytes.NewReader(data))
			if err != nil {
				return fmt.Errorf("creating XZ stream: %w", err)
			}
			streamType = "XZ+tar"
		} else if data[0] == 0x1F && data[1] == 0x8B {
			// Check for gzip format (magic bytes: 0x1F 0x8B)
			stream, err = fd.createGzipStream(bytes.NewReader(data))
			if err != nil {
				return fmt.Errorf("creating gzip stream: %w", err)
			}
			streamType = "gzip+tar"
		}
	}

	if stream == nil {
		// Try XZ as fallback
		stream, err = fd.createXZStream(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("creating XZ stream (fallback): %w", err)
		}
		streamType = "XZ+tar (fallback)"
	}

	fmt.Printf("  → Detected %s archive, extracting\n", streamType)
	return fd.extractTar(stream, destDir)
}

// generateRuntimeName creates a smart runtime name based on version and build type
func (fd *FactorioDownloader) generateRuntimeName(version, buildType string) string {
	// If build type is "alpha" (default), just use the version
	if buildType == "alpha" {
		return version
	}

	// For other build types, append the build type
	return fmt.Sprintf("%s-%s", version, buildType)
}

// createXZStream creates a stream that decompresses XZ data using pure Go
func (fd *FactorioDownloader) createXZStream(reader io.Reader) (io.Reader, error) {
	xzReader, err := xz.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("creating XZ reader: %w", err)
	}
	return xzReader, nil
}

// createGzipStream creates a stream that decompresses gzip data
func (fd *FactorioDownloader) createGzipStream(reader io.Reader) (io.Reader, error) {
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("creating gzip reader: %w", err)
	}
	return gzReader, nil
}

// extractTarGz extracts a tar.gz archive
func (fd *FactorioDownloader) extractTarGz(reader io.Reader, destDir string) error {
	// Read all data first to handle potential HTTP gzip encoding
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("reading archive data: %w", err)
	}

	// Debug: Check the first few bytes to see what we got
	if len(data) > 10 {
		fmt.Printf("  → Debug: First 10 bytes: %v\n", data[:10])
	}

	// Create a reader from the data
	dataReader := bytes.NewReader(data)

	gzReader, err := gzip.NewReader(dataReader)
	if err != nil {
		// If gzip fails, try treating it as a regular tar file
		fmt.Printf("  → Debug: Gzip failed, trying as regular tar: %v\n", err)
		return fd.extractTar(dataReader, destDir)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Skip the root directory entry
		if header.Name == "." || header.Name == "./" {
			continue
		}

		// Remove leading "./" if present
		path := strings.TrimPrefix(header.Name, "./")
		targetPath := filepath.Join(destDir, path)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			// Create parent directories
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", targetPath, err)
			}

			// Create file
			file, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("creating file %s: %w", targetPath, err)
			}

			// Copy file contents
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("copying file %s: %w", targetPath, err)
			}
			file.Close()

			// Set file permissions
			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("setting permissions for %s: %w", targetPath, err)
			}
		}
	}

	return nil
}

// extractTar extracts a regular tar archive (without gzip)
func (fd *FactorioDownloader) extractTar(reader io.Reader, destDir string) error {
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Skip the root directory entry
		if header.Name == "." || header.Name == "./" {
			continue
		}

		// Remove leading "./" if present
		path := strings.TrimPrefix(header.Name, "./")
		targetPath := filepath.Join(destDir, path)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			// Create parent directories
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", targetPath, err)
			}

			// Create file
			file, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("creating file %s: %w", targetPath, err)
			}

			// Copy file contents
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("copying file %s: %w", targetPath, err)
			}
			file.Close()

			// Set file permissions
			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("setting permissions for %s: %w", targetPath, err)
			}
		}
	}

	return nil
}

// extractDMG extracts a macOS DMG file by mounting it and copying the Factorio.app
func (fd *FactorioDownloader) extractDMG(reader io.Reader, destDir string) error {
	// Create a temporary file for the DMG
	tmpFile, err := os.CreateTemp("", "factorio-*.dmg")
	if err != nil {
		return fmt.Errorf("creating temporary DMG file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write the DMG data to the temporary file
	if _, err := io.Copy(tmpFile, reader); err != nil {
		return fmt.Errorf("writing DMG data: %w", err)
	}
	tmpFile.Close()

	// Create a temporary mount point
	mountPoint, err := os.MkdirTemp("", "factorio-mount-*")
	if err != nil {
		return fmt.Errorf("creating mount point: %w", err)
	}
	defer os.RemoveAll(mountPoint)

	// Mount the DMG
	cmd := exec.Command("hdiutil", "attach", tmpFile.Name(), "-mountpoint", mountPoint, "-nobrowse", "-quiet")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mounting DMG: %w", err)
	}

	// Unmount the DMG when done
	defer func() {
		exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
	}()

	// Look for Factorio.app in the mounted DMG
	factorioAppPath := filepath.Join(mountPoint, "Factorio.app")
	if _, err := os.Stat(factorioAppPath); err != nil {
		// Try alternative locations
		altPaths := []string{
			filepath.Join(mountPoint, "Applications", "Factorio.app"),
			filepath.Join(mountPoint, "factorio", "Factorio.app"),
		}

		found := false
		for _, altPath := range altPaths {
			if _, err := os.Stat(altPath); err == nil {
				factorioAppPath = altPath
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("Factorio.app not found in DMG")
		}
	}

	// Copy Factorio.app to the destination
	destAppPath := filepath.Join(destDir, "Factorio.app")
	if err := fd.copyDirectory(factorioAppPath, destAppPath); err != nil {
		return fmt.Errorf("copying Factorio.app: %w", err)
	}

	fmt.Printf("  → Extracted Factorio.app from DMG\n")
	return nil
}

// copyDirectory recursively copies a directory
func (fd *FactorioDownloader) copyDirectory(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	// Read source directory
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := fd.copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := fd.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func (fd *FactorioDownloader) copyFile(src, dst string) error {
	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy file contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Copy file permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}
	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		return err
	}

	return nil
}

// extractZIP extracts a ZIP archive
func (fd *FactorioDownloader) extractZIP(reader io.Reader, destDir string) error {
	// Read all data into memory for ZIP processing
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("reading ZIP data: %w", err)
	}

	// Create a ZIP reader
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("creating ZIP reader: %w", err)
	}

	// Extract all files
	for _, file := range zipReader.File {
		path := filepath.Join(destDir, file.Name)

		// Skip directories (they'll be created when we process files)
		if file.FileInfo().IsDir() {
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", path, err)
		}

		// Open file in ZIP
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("opening file %s in ZIP: %w", file.Name, err)
		}

		// Create target file
		targetFile, err := os.Create(path)
		if err != nil {
			rc.Close()
			return fmt.Errorf("creating file %s: %w", path, err)
		}

		// Copy file contents
		if _, err := io.Copy(targetFile, rc); err != nil {
			rc.Close()
			targetFile.Close()
			return fmt.Errorf("copying file %s: %w", path, err)
		}

		rc.Close()
		targetFile.Close()

		// Set file permissions
		if err := os.Chmod(path, file.FileInfo().Mode()); err != nil {
			return fmt.Errorf("setting permissions for %s: %w", path, err)
		}
	}

	return nil
}

// GetLatestVersion retrieves the latest Factorio version for a specific build type
func (fd *FactorioDownloader) GetLatestVersion(ctx context.Context, buildType string, allowExperimental bool) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://factorio.com/api/latest-releases", nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching latest releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch latest releases: status %d", resp.StatusCode)
	}

	// Parse the response to get the latest version
	// The API returns JSON with experimental and stable sections
	var releases struct {
		Experimental struct {
			Alpha     string `json:"alpha"`
			Demo      string `json:"demo"`
			Expansion string `json:"expansion"`
			Headless  string `json:"headless"`
		} `json:"experimental"`
		Stable struct {
			Alpha     string `json:"alpha"`
			Demo      string `json:"demo"`
			Expansion string `json:"expansion"`
			Headless  string `json:"headless"`
		} `json:"stable"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("parsing releases JSON: %w", err)
	}

	// Choose between experimental and stable based on allowExperimental flag
	var version string
	var source string

	if allowExperimental {
		// Use experimental versions (latest features)
		switch buildType {
		case "alpha":
			version = releases.Experimental.Alpha
		case "headless":
			version = releases.Experimental.Headless
		case "expansion":
			version = releases.Experimental.Expansion
		case "demo":
			version = releases.Experimental.Demo
		default:
			return "", fmt.Errorf("unsupported build type: %s", buildType)
		}
		source = "experimental"
	} else {
		// Use stable versions (reliable)
		switch buildType {
		case "alpha":
			version = releases.Stable.Alpha
		case "headless":
			version = releases.Stable.Headless
		case "expansion":
			version = releases.Stable.Expansion
		case "demo":
			version = releases.Stable.Demo
		default:
			return "", fmt.Errorf("unsupported build type: %s", buildType)
		}
		source = "stable"
	}

	if version == "" {
		return "", fmt.Errorf("no %s %s version found in releases", source, buildType)
	}

	fmt.Printf("  → Using %s %s version: %s\n", source, buildType, version)
	return version, nil
}
