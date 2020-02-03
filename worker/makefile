all: tsunami

clean:
	rm tsunami

tsunami:
	go build tsunami.go server.go json.go shell.go help.go job.go worker.go 

install:
	cp -f tsunami /usr/local/bin
