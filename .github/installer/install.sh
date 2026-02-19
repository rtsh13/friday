#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────
#  Friday - Telemetry Debugger Installer
#  Supports: Linux (amd64), macOS (amd64 + arm64)
# ─────────────────────────────────────────────────────────────

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="$HOME/.friday"
LIB_DIR="$HOME/.friday/lib"
BIN_DIR="$HOME/.friday/bin"
BINARY_NAME="friday"
COMPOSE_PROJECT_NAME="friday"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC}  $1"; }
log_success() { echo -e "${GREEN}[OK]${NC}    $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC}  $1"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1"; }

echo ""
echo "╔════════════════════════════════════════════╗"
echo "║   Friday - Telemetry Debugger Installer    ║"
echo "╚════════════════════════════════════════════╝"
echo ""

# ── 1. Detect OS and architecture ─────────────────────────────
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  PLATFORM="linux" ;;
  Darwin) PLATFORM="darwin" ;;
  *)
    log_error "Unsupported operating system: $OS"
    log_error "This installer supports Linux and macOS only."
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64)        ARCH_LABEL="amd64" ;;
  arm64|aarch64) ARCH_LABEL="arm64" ;;
  *)
    log_error "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

log_info "Detected platform: $PLATFORM/$ARCH_LABEL"

# ── 2. Check Docker is installed ──────────────────────────────
log_info "Checking Docker installation..."
if ! command -v docker &>/dev/null; then
  log_error "Docker is not installed or not in PATH."
  log_error "Please install Docker Desktop from https://www.docker.com/products/docker-desktop/"
  exit 1
fi
log_success "Docker found: $(docker --version)"

# ── 2b. Check docker compose v2 is available ──────────────────
log_info "Checking docker compose v2..."
if ! docker compose version &>/dev/null; then
  log_error "docker compose v2 (plugin) is not available."
  log_error "Please update Docker to a version that includes the compose plugin."
  log_error "See: https://docs.docker.com/compose/install/"
  exit 1
fi
log_success "docker compose v2 found."

# ── 3. Check Docker is running ────────────────────────────────
log_info "Checking Docker daemon..."
if ! docker info &>/dev/null; then
  log_error "Docker daemon is not running. Please start Docker Desktop and re-run this installer."
  exit 1
fi
log_success "Docker daemon is running."

# ── 4. Check NVIDIA GPU availability ─────────────────────────
log_info "Checking for NVIDIA GPU..."
if ! command -v nvidia-smi &>/dev/null; then
  log_warn "nvidia-smi not found. vLLM requires an NVIDIA GPU with CUDA support."
  log_warn "If you are running on a machine without an NVIDIA GPU, Friday will not work."
  read -rp "Continue anyway? [y/N]: " gpu_confirm
  if [[ ! "$gpu_confirm" =~ ^[Yy]$ ]]; then
    log_info "Installation cancelled."
    exit 0
  fi
else
  log_success "NVIDIA GPU detected: $(nvidia-smi --query-gpu=name --format=csv,noheader | head -1)"

  # Verify Docker can access the GPU via NVIDIA Container Toolkit
  log_info "Checking NVIDIA Container Toolkit for Docker..."
  if ! docker run --rm --gpus all nvidia/cuda:12.0-base-ubuntu22.04 nvidia-smi &>/dev/null; then
    log_warn "Docker cannot access your NVIDIA GPU."
    log_warn "The NVIDIA Container Toolkit may not be installed or configured."
    log_warn "See: https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html"
    read -rp "Continue anyway? [y/N]: " toolkit_confirm
    if [[ ! "$toolkit_confirm" =~ ^[Yy]$ ]]; then
      log_info "Installation cancelled."
      exit 0
    fi
  else
    log_success "Docker GPU access confirmed."
  fi
fi

# ── 5. Select correct binary and library ──────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_PATH="$SCRIPT_DIR/bin/${BINARY_NAME}-${PLATFORM}-${ARCH_LABEL}"

if [[ ! -f "$BINARY_PATH" ]]; then
  log_error "Binary not found: $BINARY_PATH"
  log_error "Expected binary for your platform is missing from the installer package."
  exit 1
fi

# Determine shared library path based on platform
if [[ "$PLATFORM" == "linux" ]]; then
  LIB_SRC="$SCRIPT_DIR/bin/libonnxruntime.so"
  LIB_DEST="$LIB_DIR/libonnxruntime.so"
  if [[ ! -f "$LIB_SRC" ]]; then
    log_error "ONNX Runtime library not found: $LIB_SRC"
    exit 1
  fi
elif [[ "$PLATFORM" == "darwin" ]]; then
  LIB_SRC="$SCRIPT_DIR/bin/libonnxruntime-${ARCH_LABEL}.dylib"
  LIB_DEST="$LIB_DIR/libonnxruntime.dylib"
  if [[ ! -f "$LIB_SRC" ]]; then
    log_error "ONNX Runtime library not found: $LIB_SRC"
    exit 1
  fi
fi

# ── 6. First-run download warning ─────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║                  FIRST-RUN NOTICE                        ║"
echo "╠══════════════════════════════════════════════════════════╣"
echo "║  On first startup, Friday will download the DocLM model  ║"
echo "║  (~6 GB) from HuggingFace. This is a one-time download.  ║"
echo "║  Subsequent runs use the locally cached model.           ║"
echo "║                                                           ║"
echo "║  Ensure you have:                                         ║"
echo "║    • A stable internet connection                         ║"
echo "║    • At least 8 GB of free disk space                    ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
read -rp "Proceed with installation? [y/N]: " install_confirm
if [[ ! "$install_confirm" =~ ^[Yy]$ ]]; then
  log_info "Installation cancelled."
  exit 0
fi

# ── 7. Create directories ─────────────────────────────────────
log_info "Creating directories..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$LIB_DIR"
mkdir -p "$BIN_DIR"
log_success "Directories created."

# ── 8. Copy config files ──────────────────────────────────────
log_info "Copying config files..."
cp "$SCRIPT_DIR/docker-compose.yml" "$CONFIG_DIR/docker-compose.yml"
cp "$SCRIPT_DIR/config.yaml"        "$CONFIG_DIR/config.yaml"
log_success "Config files copied to $CONFIG_DIR"

# ── 9. Install binary ─────────────────────────────────────────
log_info "Installing friday binary..."
cp "$BINARY_PATH" "$BIN_DIR/friday-bin"
chmod +x "$BIN_DIR/friday-bin"
log_success "Binary installed to $BIN_DIR/friday-bin"

# ── 10. Install ONNX Runtime shared library ───────────────────
log_info "Installing ONNX Runtime shared library..."
cp "$LIB_SRC" "$LIB_DEST"
log_success "ONNX Runtime library installed to $LIB_DEST"

# ── 11. Create wrapper script ─────────────────────────────────
# The wrapper sets the shared library path so the binary can find
# libonnxruntime at runtime without system-wide installation.
log_info "Creating wrapper script..."

if [[ "$PLATFORM" == "linux" ]]; then
  WRAPPER_CONTENT="#!/usr/bin/env bash
export LD_LIBRARY_PATH=\"$LIB_DIR:\$LD_LIBRARY_PATH\"
exec \"$BIN_DIR/friday-bin\" \"\$@\"
"
elif [[ "$PLATFORM" == "darwin" ]]; then
  WRAPPER_CONTENT="#!/usr/bin/env bash
export DYLD_LIBRARY_PATH=\"$LIB_DIR:\$DYLD_LIBRARY_PATH\"
exec \"$BIN_DIR/friday-bin\" \"\$@\"
"
fi

WRAPPER_PATH="$BIN_DIR/friday"
echo "$WRAPPER_CONTENT" > "$WRAPPER_PATH"
chmod +x "$WRAPPER_PATH"
log_success "Wrapper script created at $WRAPPER_PATH"

# ── 12. Symlink wrapper to /usr/local/bin ─────────────────────
log_info "Linking friday to $INSTALL_DIR..."
if [[ ! -w "$INSTALL_DIR" ]]; then
  log_info "Elevated permissions required to install to $INSTALL_DIR"
  sudo ln -sf "$WRAPPER_PATH" "$INSTALL_DIR/$BINARY_NAME"
else
  ln -sf "$WRAPPER_PATH" "$INSTALL_DIR/$BINARY_NAME"
fi
log_success "friday is now available system-wide."

# ── 13. Pull Docker images ────────────────────────────────────
log_info "Pulling Docker images (qdrant + vllm:v0.8.5)..."
log_info "This may take a few minutes depending on your connection..."
docker compose -f "$CONFIG_DIR/docker-compose.yml" -p "$COMPOSE_PROJECT_NAME" pull
log_success "Docker images pulled successfully."

# ── 14. Start services ────────────────────────────────────────
log_info "Starting Friday services..."
docker compose -f "$CONFIG_DIR/docker-compose.yml" -p "$COMPOSE_PROJECT_NAME" up -d
log_success "Services started."

# ── 15. Done ──────────────────────────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║              Installation Complete!                      ║"
echo "╠══════════════════════════════════════════════════════════╣"
echo "║  NOTE: The DocLM model (~6 GB) is downloading in the     ║"
echo "║  background. Friday will be ready once the download      ║"
echo "║  completes. This is a one-time process.                  ║"
echo "║                                                           ║"
echo "║  Check model status:                                      ║"
echo "║    docker logs -f friday-vllm-1                          ║"
echo "║                                                           ║"
echo "║  Once ready, run:                                         ║"
echo "║    friday                                                 ║"
echo "║                                                           ║"
echo "║  To stop services:                                        ║"
echo "║    docker compose -f ~/.friday/docker-compose.yml down   ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
