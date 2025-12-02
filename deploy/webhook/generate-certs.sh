#!/bin/bash

# Script to generate TLS certificates for VPSie Autoscaler webhook
# This script generates a self-signed CA and server certificate for the webhook

set -e

NAMESPACE=${NAMESPACE:-kube-system}
SERVICE_NAME=${SERVICE_NAME:-vpsie-autoscaler-webhook}
SECRET_NAME=${SECRET_NAME:-vpsie-autoscaler-webhook-certs}
WEBHOOK_CONFIG_NAME=${WEBHOOK_CONFIG_NAME:-vpsie-autoscaler-webhook}

echo "Generating TLS certificates for webhook..."
echo "  Namespace: ${NAMESPACE}"
echo "  Service: ${SERVICE_NAME}"
echo "  Secret: ${SECRET_NAME}"

# Create temporary directory for certificates
CERT_DIR=$(mktemp -d)
trap "rm -rf ${CERT_DIR}" EXIT

cd ${CERT_DIR}

# Generate CA key and certificate
echo "Generating CA certificate..."
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -days 3650 -out ca.crt -subj "/CN=vpsie-autoscaler-webhook-ca"

# Generate server key
echo "Generating server key..."
openssl genrsa -out tls.key 2048

# Create certificate signing request configuration
cat > csr.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${SERVICE_NAME}
DNS.2 = ${SERVICE_NAME}.${NAMESPACE}
DNS.3 = ${SERVICE_NAME}.${NAMESPACE}.svc
DNS.4 = ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

# Generate certificate signing request
echo "Generating CSR..."
openssl req -new -key tls.key -out tls.csr -subj "/CN=${SERVICE_NAME}.${NAMESPACE}.svc" -config csr.conf

# Sign the certificate with CA
echo "Signing certificate..."
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 3650 -extensions v3_req -extfile csr.conf

# Create Kubernetes secret
echo "Creating Kubernetes secret..."
kubectl create secret generic ${SECRET_NAME} \
  --from-file=tls.key=tls.key \
  --from-file=tls.crt=tls.crt \
  --from-file=ca.crt=ca.crt \
  -n ${NAMESPACE} \
  --dry-run=client -o yaml | kubectl apply -f -

# Get CA bundle for webhook configuration
CA_BUNDLE=$(base64 < ca.crt | tr -d '\n')

# Update ValidatingWebhookConfiguration with CA bundle
echo "Updating ValidatingWebhookConfiguration..."
kubectl patch validatingwebhookconfiguration ${WEBHOOK_CONFIG_NAME} \
  --type='json' \
  -p='[
    {
      "op": "replace",
      "path": "/webhooks/0/clientConfig/caBundle",
      "value": "'"${CA_BUNDLE}"'"
    },
    {
      "op": "replace",
      "path": "/webhooks/1/clientConfig/caBundle",
      "value": "'"${CA_BUNDLE}"'"
    }
  ]' 2>/dev/null || echo "Warning: Failed to patch webhook configuration. You may need to manually update the caBundle."

echo ""
echo "✅ TLS certificates generated successfully!"
echo ""
echo "Secret '${SECRET_NAME}' created in namespace '${NAMESPACE}'"
echo ""
echo "If the webhook configuration was not updated automatically, please update it manually:"
echo "  caBundle: ${CA_BUNDLE}"
echo ""
echo "Certificate details:"
echo "  Valid for: 3650 days (10 years)"
echo "  Subject: CN=${SERVICE_NAME}.${NAMESPACE}.svc"
echo "  SANs:"
echo "    - ${SERVICE_NAME}"
echo "    - ${SERVICE_NAME}.${NAMESPACE}"
echo "    - ${SERVICE_NAME}.${NAMESPACE}.svc"
echo "    - ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local"
echo ""
