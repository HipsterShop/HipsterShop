#!/bin/bash

# Define the list of Docker images
IMAGES=(
    "cazzzzz/trf-adservice-hipster:latest"
    "cazzzzz/trf-assistantservice-hipster:latest"
    "cazzzzz/trf-authservice-hipster:latest"
    "cazzzzz/trf-cartservice-hipster:latest"
    "cazzzzz/trf-checkoutservice-hipster:latest"
    "cazzzzz/trf-currencyservice-hipster:latest"
    "cazzzzz/trf-emailservice-hipster:latest"
    "cazzzzz/trf-frontend-hipster:latest"
    "cazzzzz/trf-paymentservice-hipster:latest"
    "cazzzzz/trf-productcatalogservice-hipster:latest"
    "cazzzzz/trf-recommendationservice-hipster:latest"
    "cazzzzz/trf-shippingservice-hipster:latest"
)

# Define the output file
RESULT_FILE="result.txt"

# Clear or create the file at the start (optional)
echo "Scan Report - $(date)" > "$RESULT_FILE"
echo "--------------------------" >> "$RESULT_FILE"

for IMAGE in "${IMAGES[@]}"; do
    echo "Currently scanning: $IMAGE"
    
    # Header for each specific image in the text file
    echo "RESULTS FOR: $IMAGE" >> "$RESULT_FILE"
    
    # Run Trivy and append both standard output and errors to the file
    trivy image "$IMAGE" >> "$RESULT_FILE" 2>&1
    
    # Add a separator for readability
    echo -e "\n--------------------------\n" >> "$RESULT_FILE"
done

echo "Scan complete. Check $RESULT_FILE for details."
