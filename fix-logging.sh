#!/bin/bash
cd "$(dirname "$0")"

# Replace logging.Logger with s.logger in all scaler package files
for file in pkg/scaler/*.go; do
    if [ -f "$file" ]; then
        sed -i '' 's/logging\.Logger/s.logger/g' "$file"
        echo "Fixed $file"
    fi
done

# Add zap import to files that need it (if not already present)
for file in pkg/scaler/safety.go pkg/scaler/policies.go pkg/scaler/utilization.go; do
    if [ -f "$file" ]; then
        # Check if zap is already imported
        if ! grep -q '"go.uber.org/zap"' "$file"; then
            # Add zap import after the logging import
            sed -i '' '/"github.com\/vpsie\/vpsie-k8s-autoscaler\/pkg\/logging"/a\
\
\	"go.uber.org/zap"
' "$file"
            echo "Added zap import to $file"
        fi
    fi
done

echo "Done!"
