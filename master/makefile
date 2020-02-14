all: ocean 

clean:
	rm ocean 

ocean:
	go build ocean.go services.go client.go 

install:
	cp -f ocean /usr/local/bin
