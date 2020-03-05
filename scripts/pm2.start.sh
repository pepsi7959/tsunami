# Service Registry
#pm2 start /usr/bin/etcd --name etcd-1 --namespace tsunami -- --name etcd-1 --listen-client-urls http://127.0.0.1:2379 --advertise-client-urls http://127.0.0.1:2379 --listen-peer-urls http://127.0.0.1:12380 --initial-advertise-peer-urls http://127.0.0.1:12380 --initial-cluster-token etcd-cluster-1 --initial-cluster 'etcd-1=http://127.0.0.1:12380,etcd-2=http://127.0.0.1:22380,etcd-3=http://127.0.0.1:32380' --initial-cluster-state new --enable-pprof
#pm2 start /usr/bin/etcd --name etcd-2 --namespace tsunami -- --name etcd-2 --listen-client-urls http://127.0.0.1:22379 --advertise-client-urls http://127.0.0.1:22379 --listen-peer-urls http://127.0.0.1:22380 --initial-advertise-peer-urls http://127.0.0.1:22380 --initial-cluster-token etcd-cluster-1 --initial-cluster 'etcd-1=http://127.0.0.1:12380,etcd-2=http://127.0.0.1:22380,etcd-3=http://127.0.0.1:32380' --initial-cluster-state new --enable-pprof
#pm2 start /usr/bin/etcd --name etcd-3 --namespace tsunami -- --name etcd-3 --listen-client-urls http://127.0.0.1:32379 --advertise-client-urls http://127.0.0.1:32379 --listen-peer-urls http://127.0.0.1:32380 --initial-advertise-peer-urls http://127.0.0.1:32380 --initial-cluster-token etcd-cluster-1 --initial-cluster 'etcd-1=http://127.0.0.1:12380,etcd-2=http://127.0.0.1:22380,etcd-3=http://127.0.0.1:32380' --initial-cluster-state new --enable-pprof
pm2 start /usr/bin/etcd --name etcd-1 --namespace tsunami -- --config-file /etc/tsunami/conf/registry/etcd-1.config.yaml
pm2 start /usr/bin/etcd --name etcd-2 --namespace tsunami -- --config-file /etc/tsunami/conf/registry/etcd-2.config.yaml
pm2 start /usr/bin/etcd --name etcd-3 --namespace tsunami -- --config-file /etc/tsunami/conf/registry/etcd-3.config.yaml

# Worker Node 
pm2 start /usr/local/bin/tsunami --name tsunami-1 --namespace tsunami -- --path /etc/tsunami/conf/tsunami --file config-1.yaml
pm2 start /usr/local/bin/tsunami --name tsunami-2 --namespace tsunami -- --path /etc/tsunami/conf/tsunami --file config-2.yaml
pm2 start /usr/local/bin/tsunami --name tsunami-3 --namespace tsunami -- --path /etc/tsunami/conf/tsunami --file config-3.yaml

# Master Node
pm2 start /usr/local/bin/ocean --name ocean --namespace tsunami -- --path /etc/tsunami/conf/ocean --file config.yaml