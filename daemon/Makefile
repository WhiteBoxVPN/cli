BINARY_NAME=wbd

all: build
 
build:
	go build -o ${BINARY_NAME} main.go
 
run:
	go build -o ${BINARY_NAME} main.go
	./${BINARY_NAME}
 
clean:
	go clean
	rm ${BINARY_NAME}

install:
	$(MAKE) build
	cp ${BINARY_NAME} /usr/local/bin
