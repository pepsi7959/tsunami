all: tsunami

clean:
	rm tsunami

tsunami:
	go build tsunami.go json.go shell.go help.go job.go worker.go utils.go

install:
	cp -f tsunami /usr/local/bin
