all: gen

clean:
	rm *.pb.go

gen:
	protoc --proto_path=./ --go_out=plugins=grpc:. services.proto
