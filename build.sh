#!/bin/bash
cd "$(dirname "$0")"
go build -o /tmp/vpsie-autoscaler ./cmd/controller
