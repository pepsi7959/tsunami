all: gen

clean:
	rm *.pb.go

gen:
	protoc --proto_path=./ \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		services.proto
