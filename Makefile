BINARY_NAME=wb

all: build
 
build:
	go build -o ${BINARY_NAME} .
 
run:
	go build -o ${BINARY_NAME} .
	./${BINARY_NAME}
 
clean:
	go clean
	rm ${BINARY_NAME}

install:
	$(MAKE) build
	cp ${BINARY_NAME} /usr/local/bin
