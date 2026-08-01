#!/bin/bash
export PATH="$HOME/go/bin:$PATH"
set -e
cd "$(dirname "$0")"

echo "========================================"
echo "  wa-api — WhatsApp QR Code Generator"
echo "========================================"

# 1. Kill any existing server
echo "[1/6] Stopping old server..."
kill $(lsof -ti:8080) 2>/dev/null || true
sleep 1

# 2. Clean database
echo "[2/6] Cleaning database..."
rm -rf dbdata wuzapi.db

# 3. Create .env if missing
if [ ! -f .env ]; then
  echo "[3/6] Creating .env..."
  cat > .env << 'ENVEOF'
WUZAPI_ADMIN_TOKEN=admin123
WUZAPI_GLOBAL_ENCRYPTION_KEY=MinhaChaveDe32Caracteres1234567890
WUZAPI_GLOBAL_HMAC_KEY=MinhaHMACKeyDe32Caracteres1234567
DB_NAME=waapi
ENVEOF
else
  echo "[3/6] .env already exists"
fi

# 4. Build
echo "[4/6] Building..."
go build -o wuzapi ./cmd/core
echo "      Binary: $(ls -lh wuzapi | awk '{print $5}')"

# 5. Start server
echo "[5/6] Starting server..."
./wuzapi --logtype=json --color=false > /tmp/wuzapi.log 2>&1 &
SERVER_PID=$!
sleep 3

if ! kill -0 $SERVER_PID 2>/dev/null; then
  echo "ERROR: Server failed to start. Check /tmp/wuzapi.log"
  exit 1
fi
echo "      Server PID: $SERVER_PID"

# 6. Create user
echo "[6/6] Creating test user..."
curl -s -X POST http://localhost:8080/admin/users \
  -H "Authorization: admin123" \
  -H "Content-Type: application/json" \
  -d '{"name":"WhatsAppBot","token":"t1"}' > /dev/null

echo ""
echo "========================================"
echo "  READY! Run the command below to get "
echo "  the QR code (appears in terminal):"
echo ""
echo "  curl -N http://localhost:8080/session/connect?token=t1"
echo ""
echo "  Or get base64 PNG for scanning:"
echo "  curl -s http://localhost:8080/session/qr?token=t1"
echo ""
echo "  Server running on http://localhost:8080"
echo "  Admin token: admin123"
echo "  Logs: tail -f /tmp/wuzapi.log"
echo "========================================"
