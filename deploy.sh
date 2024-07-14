#!/bin/bash

# Step 1: Build the Go binary
echo "Building Go binary..."
GOOS=linux GOARCH=amd64 go build  -o reverse_proxy ./server.go|| { echo "Go build failed"; exit 1; }

# Step 2: SCP the Go binary to the EC2 instance
echo "Copying Go binary to EC2 instance..."
scp -i $EC2_PEM_PATH reverse_proxy ubuntu@$EC2_PUBLIC_IP:/home/ubuntu/.temp/ || { echo "SCP failed"; exit 1; }

# Step 3: SSH into the EC2 instance and move the file
echo "Connecting to EC2 instance and moving the file..."
ssh -i $EC2_PEM_PATH ubuntu@$EC2_PUBLIC_IP << EOF
  mv ./.temp/reverse_proxy . || { echo "Failed to move the file"; exit 1; }
  chmod +x reverse_proxy || { echo "Failed to change permissions"; exit 1; }
  echo "File moved successfully"
  sudo systemctl restart reverse_proxy || { echo "Failed to restart"; exit 1; }
EOF

echo "Script completed successfully."
rm reverse_proxy