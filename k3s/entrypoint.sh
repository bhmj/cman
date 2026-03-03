#!/bin/bash
set -e

DOCKER_WAIT_TIMEOUT=${DOCKER_WAIT_TIMEOUT:-60}
BUILD_DIR=/app/docker-assets

echo "Waiting for Docker daemon (timeout: ${DOCKER_WAIT_TIMEOUT}s)..."
elapsed=0
until docker info >/dev/null 2>&1; do
    if [ "$elapsed" -ge "$DOCKER_WAIT_TIMEOUT" ]; then
        echo "ERROR: Docker daemon did not become ready within ${DOCKER_WAIT_TIMEOUT}s"
        exit 1
    fi
    sleep 1
    elapsed=$((elapsed + 1))
done
echo "Docker daemon is ready (took ${elapsed}s)."

build_playground_images() {
    echo "Checking playground images..."
    cd "$BUILD_DIR"
    for FILE in Dockerfile.*_*; do
        [ -f "$FILE" ] || continue
        ITEM=${FILE#Dockerfile.}
        IFS="_" read -r NAME VERSION <<< "$ITEM"
        IMAGE_NAME="combobox-${NAME}:${VERSION}"
        if docker image inspect "$IMAGE_NAME" >/dev/null 2>&1; then
            echo "  $IMAGE_NAME: exists"
        else
            echo "  $IMAGE_NAME: building..."
            docker build -f "$FILE" -t "$IMAGE_NAME" . || {
                echo "  WARNING: failed to build $IMAGE_NAME"
                continue
            }
            echo "  $IMAGE_NAME: built OK"
        fi
    done
    cd /app
}

build_playground_images

echo "Starting CMan..."
exec /app/cman --config-file=/app/config/config.yaml