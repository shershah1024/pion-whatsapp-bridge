# Installation Guide

## Installing Go

Since Go is not currently installed on your system, here's how to install it:

### Option 1: Using Homebrew (Recommended for macOS)

```bash
# Install Homebrew if you don't have it
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install Go
brew install go

# Verify installation
go version
```

### Option 2: Direct Download

1. Visit https://golang.org/dl/
2. Download the macOS installer (darwin-arm64 for M1/M2 Macs)
3. Run the installer
4. Add to PATH in ~/.zshrc or ~/.bash_profile:
   ```bash
   export PATH=$PATH:/usr/local/go/bin
   ```

### Option 3: Using MacPorts

```bash
sudo port install go
```

## Installing ngrok

### Using Homebrew

```bash
brew install ngrok
```

### Direct Download

1. Visit https://ngrok.com/download
2. Download for macOS
3. Unzip and move to /usr/local/bin:
   ```bash
   unzip ngrok-stable-darwin-arm64.zip
   sudo mv ngrok /usr/local/bin/
   ```

## Quick Verification

After installation, verify both tools:

```bash
# Check Go
go version

# Check ngrok
ngrok version
```

## Building and Running

Once Go is installed:

```bash
# Download dependencies
go mod download

# Build the application
go build -o pion-whatsapp-bridge .

# Run the deployment
./deploy.sh
```

## Troubleshooting

### Go not found after installation
- Close and reopen your terminal
- Check PATH: `echo $PATH`
- Source your profile: `source ~/.zshrc`

### ngrok authentication
- Sign up at https://ngrok.com
- Run: `ngrok authtoken YOUR_AUTH_TOKEN`

### Build errors
- Ensure Go 1.21+ is installed: `go version`
- Clear module cache: `go clean -modcache`
- Re-download dependencies: `go mod download`