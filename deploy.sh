#!/bin/bash

# Step 1: Build the Go binary
echo "Building Go binary..."
GOOS=linux GOARCH=amd64 go build ./server.go -o main || { echo "Go build failed"; exit 1; }

# Step 2: SCP the Go binary to the EC2 instance
echo "Copying Go binary to EC2 instance..."
scp -i /Users/jacksonstone/Desktop/Jackson\ Personal\ Site\ Key.pem main ubuntu@3.19.146.227:/home/ubuntu/.temp/ || { echo "SCP failed"; exit 1; }

# Step 3: SSH into the EC2 instance and move the file
echo "Connecting to EC2 instance and moving the file..."
ssh -i /Users/jacksonstone/Desktop/Jackson\ Personal\ Site\ Key.pem ubuntu@3.19.146.227 << EOF
  mv ./.temp/main . || { echo "Failed to move the file"; exit 1; }
  echo "File moved successfully"
  sudo systemctl restart server || { echo "Failed to restart"; exit 1; }
EOF

echo "Script completed successfully."