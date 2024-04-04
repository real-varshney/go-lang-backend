# Use the official Golang image as a parent image
FROM golang:1.19

# Set the working directory inside the container
WORKDIR /app

# Copy the go.mod and go.sum files into the container
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the rest of the source code into the container
COPY . .

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/exploding-kitten

# Expose port 8080 for the application to listen on
EXPOSE 8080

# Command to run the application
CMD ["/app/exploding-kitten"]
