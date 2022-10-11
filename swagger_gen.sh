#!/usr/bin/env bash
set -ue

swag init -g admin/admin.go -d ./internal -o ./internal/admin/docs --parseInternal  --generatedTime
