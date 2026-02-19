#Requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ─────────────────────────────────────────────────────────────
#  Friday - Telemetry Debugger Installer (Windows)
# ─────────────────────────────────────────────────────────────

$BinaryName     = "friday.exe"
$OnnxDllName    = "onnxruntime.dll"
$ConfigDir      = "$env:USERPROFILE\.friday"
$InstallDir     = "$env:USERPROFILE\.friday\bin"
$ComposeProject = "friday"

function Write-Info    { param($msg) Write-Host "[INFO]  $msg" -ForegroundColor Cyan }
function Write-Success { param($msg) Write-Host "[OK]    $msg" -ForegroundColor Green }
function Write-Warn    { param($msg) Write-Host "[WARN]  $msg" -ForegroundColor Yellow }
function Write-Fail    { param($msg) Write-Host "[ERROR] $msg" -ForegroundColor Red }

Write-Host ""
Write-Host "╔════════════════════════════════════════════╗" -ForegroundColor Blue
Write-Host "║   Friday - Telemetry Debugger Installer    ║" -ForegroundColor Blue
Write-Host "╚════════════════════════════════════════════╝" -ForegroundColor Blue
Write-Host ""

# ── 1. Check Docker is installed ──────────────────────────────
Write-Info "Checking Docker installation..."
try {
    $dockerVersion = docker --version 2>&1
    Write-Success "Docker found: $dockerVersion"
} catch {
    Write-Fail "Docker is not installed or not in PATH."
    Write-Fail "Please install Docker Desktop from https://www.docker.com/products/docker-desktop/"
    exit 1
}

# ── 1b. Check docker compose v2 is available ──────────────────
Write-Info "Checking docker compose v2..."
docker compose version 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Fail "docker compose v2 (plugin) is not available."
    Write-Fail "Please update Docker Desktop to a version that includes the compose plugin."
    Write-Fail "See: https://docs.docker.com/compose/install/"
    exit 1
}
Write-Success "docker compose v2 found."

# ── 2. Check Docker is running ────────────────────────────────
Write-Info "Checking Docker daemon..."
docker info 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Fail "Docker daemon is not running. Please start Docker Desktop and re-run this installer."
    exit 1
}
Write-Success "Docker daemon is running."

# ── 3. Check NVIDIA GPU ───────────────────────────────────────
Write-Info "Checking for NVIDIA GPU..."
try {
    $gpuName = nvidia-smi --query-gpu=name --format=csv,noheader 2>&1 | Select-Object -First 1
    Write-Success "NVIDIA GPU detected: $gpuName"
} catch {
    Write-Warn "nvidia-smi not found. vLLM requires an NVIDIA GPU with CUDA support."
    Write-Warn "If this machine does not have an NVIDIA GPU, Friday will not work."
    $gpuConfirm = Read-Host "Continue anyway? [y/N]"
    if ($gpuConfirm -notmatch '^[Yy]$') {
        Write-Info "Installation cancelled."
        exit 0
    }
}

# ── 4. Locate binary and DLL ──────────────────────────────────
$ScriptDir  = Split-Path -Parent $MyInvocation.MyCommand.Definition
$BinaryPath = Join-Path $ScriptDir "bin\$BinaryName"
$DllPath    = Join-Path $ScriptDir "bin\$OnnxDllName"

if (-not (Test-Path $BinaryPath)) {
    Write-Fail "Binary not found: $BinaryPath"
    Write-Fail "Expected friday.exe is missing from the installer package."
    exit 1
}

if (-not (Test-Path $DllPath)) {
    Write-Fail "ONNX Runtime DLL not found: $DllPath"
    Write-Fail "Expected onnxruntime.dll is missing from the installer package."
    exit 1
}

# ── 5. First-run download warning ─────────────────────────────
Write-Host ""
Write-Host "╔══════════════════════════════════════════════════════════╗" -ForegroundColor Yellow
Write-Host "║                  FIRST-RUN NOTICE                        ║" -ForegroundColor Yellow
Write-Host "╠══════════════════════════════════════════════════════════╣" -ForegroundColor Yellow
Write-Host "║  On first startup, Friday will download the DocLM model  ║" -ForegroundColor Yellow
Write-Host "║  (~6 GB) from HuggingFace. This is a one-time download.  ║" -ForegroundColor Yellow
Write-Host "║  Subsequent runs use the locally cached model.           ║" -ForegroundColor Yellow
Write-Host "║                                                           ║" -ForegroundColor Yellow
Write-Host "║  Ensure you have:                                         ║" -ForegroundColor Yellow
Write-Host "║    • A stable internet connection                         ║" -ForegroundColor Yellow
Write-Host "║    • At least 8 GB of free disk space                    ║" -ForegroundColor Yellow
Write-Host "╚══════════════════════════════════════════════════════════╝" -ForegroundColor Yellow
Write-Host ""
$installConfirm = Read-Host "Proceed with installation? [y/N]"
if ($installConfirm -notmatch '^[Yy]$') {
    Write-Info "Installation cancelled."
    exit 0
}

# ── 6. Create directories ─────────────────────────────────────
Write-Info "Creating directories..."
New-Item -ItemType Directory -Force -Path $ConfigDir  | Out-Null
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Write-Success "Directories created."

# ── 7. Copy config files ──────────────────────────────────────
Write-Info "Copying config files..."
Copy-Item "$ScriptDir\docker-compose.yml" "$ConfigDir\docker-compose.yml" -Force
Copy-Item "$ScriptDir\config.yaml"        "$ConfigDir\config.yaml"        -Force
Write-Success "Config files copied to $ConfigDir"

# ── 8. Install binary and DLL ─────────────────────────────────
# onnxruntime.dll MUST be in the same directory as friday.exe
# so the runtime linker can find it.
Write-Info "Installing friday.exe and onnxruntime.dll..."
Copy-Item $BinaryPath "$InstallDir\$BinaryName" -Force
Copy-Item $DllPath    "$InstallDir\$OnnxDllName" -Force
Write-Success "Binary and DLL installed to $InstallDir"

# ── 9. Add InstallDir to user PATH ────────────────────────────
Write-Info "Checking PATH..."
$currentPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable(
        "Path",
        "$currentPath;$InstallDir",
        "User"
    )
    Write-Success "Added $InstallDir to user PATH."
    Write-Warn "Restart your terminal after installation for PATH changes to take effect."
} else {
    Write-Success "$InstallDir already in PATH."
}

# ── 10. Pull Docker images ────────────────────────────────────
Write-Info "Pulling Docker images (qdrant + vllm:v0.8.5)..."
Write-Info "This may take a few minutes depending on your connection..."
docker compose -f "$ConfigDir\docker-compose.yml" -p $ComposeProject pull
Write-Success "Docker images pulled successfully."

# ── 11. Start services ────────────────────────────────────────
Write-Info "Starting Friday services..."
docker compose -f "$ConfigDir\docker-compose.yml" -p $ComposeProject up -d
Write-Success "Services started."

# ── 12. Done ──────────────────────────────────────────────────
Write-Host ""
Write-Host "╔══════════════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║              Installation Complete!                      ║" -ForegroundColor Green
Write-Host "╠══════════════════════════════════════════════════════════╣" -ForegroundColor Green
Write-Host "║  NOTE: The DocLM model (~6 GB) is downloading in the     ║" -ForegroundColor Green
Write-Host "║  background. Friday will be ready once the download      ║" -ForegroundColor Green
Write-Host "║  completes. This is a one-time process.                  ║" -ForegroundColor Green
Write-Host "║                                                           ║" -ForegroundColor Green
Write-Host "║  Check model status:                                      ║" -ForegroundColor Green
Write-Host "║    docker logs -f friday-vllm-1                          ║" -ForegroundColor Green
Write-Host "║                                                           ║" -ForegroundColor Green
Write-Host "║  Once ready, open a NEW terminal and run:                ║" -ForegroundColor Green
Write-Host "║    friday                                                 ║" -ForegroundColor Green
Write-Host "║                                                           ║" -ForegroundColor Green
Write-Host "║  To stop services:                                        ║" -ForegroundColor Green
Write-Host "║    docker compose -f %USERPROFILE%\.friday\              ║" -ForegroundColor Green
Write-Host "║    docker-compose.yml down                                ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""
