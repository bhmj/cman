#!/bin/bash

DIR=$(dirname $0)

# Generate LIST dynamically from Dockerfiles in the current directory
LIST=()
for FILE in Dockerfile.*_*; do
    if [[ -f "$FILE" ]]; then
        ITEM=${FILE#Dockerfile.}  # Remove "Dockerfile." prefix
        LIST+=("$ITEM")
    fi
done

if [[ "$1" == "" ]]; then
    printf "Usage: $0 run\n"
    printf "Builds images defined in Dockerfile.* files in the current directory\n"
    printf "for the CMan Playground, based on contents of the 'existing.txt' file.\n"
    printf "Remove the line from 'existing.txt' to rebuild the image.\n"
    exit 0
fi

printf "\n"
printf "WARNING! This script stops running CMan containers!\n"
printf "WARNING! This script deletes updated CMan images!\n"
printf "\n"
printf "Press Ctrl+C to cancel or Enter to continue\n"
read

# File containing existing entries
EXISTING_FILE="existing.txt"

touch "$EXISTING_FILE"

# Read existing entries into an array
EXISTING_ENTRIES=()
while IFS= read -r line; do
    EXISTING_ENTRIES+=("$line")
done < "$EXISTING_FILE"

# Convert to a space-separated string for easy searching
EXISTING_SET=" ${EXISTING_ENTRIES[*]} "

echo "$EXISTING_SET"

for ITEM in "${LIST[@]}"; do
    # Check if ITEM is in EXISTING_SET
    if [[ ! " $EXISTING_SET " =~ " $ITEM " ]]; then
        IFS="_" read -r NAME VERSION <<< "$ITEM"
        
        echo "Processing $ITEM..."
        echo "<${NAME}> <${VERSION}>"
        
        # Stop and remove matching containers
        for CONTAINER in $(docker ps -a --format "{{.Names}}" | grep -E "^cman-${NAME}-${VERSION}-"); do
            echo "Stopping and removing container: $CONTAINER"
            docker stop "$CONTAINER"
            docker rm "$CONTAINER"
        done
        
        # Remove the corresponding Docker image
        IMAGE_NAME="combobox-${NAME}:${VERSION}"
        if docker images --format "{{.Repository}}:{{.Tag}}" | grep -q "^${IMAGE_NAME}$"; then
            echo "Removing image: $IMAGE_NAME"
            docker rmi "$IMAGE_NAME"
        fi
        
        # Build the new image
        DOCKERFILE="Dockerfile.${NAME}_${VERSION}"
        if [[ -f "$DOCKERFILE" ]]; then
            echo "Building new image: $IMAGE_NAME using $DOCKERFILE"
            docker build -f "$DOCKERFILE" -t "$IMAGE_NAME" .
            [ $? -ne 0 ] && continue
        else
            echo "Dockerfile $DOCKERFILE not found, skipping build."
        fi
        
        # Append to the existing file
        echo "$ITEM" >> "$EXISTING_FILE"
    else
        printf "skipping $ITEM \t : already exists in $EXISTING_FILE\n"
    fi
done
