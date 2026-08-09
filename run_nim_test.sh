#!/bin/bash
set -a
source .provider-secrets.env
set +a
go run cmd/phase415_nim_large_models_fire_test/main.go
