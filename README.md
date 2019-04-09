# Tsunami
It's load generator platform designed to test performance. In case of tuning system or finding limtation of a system, we need comfortable and stable tools. It's developed by Golang, which uses less memory footprint.

----

## Prerequisite for a client

  For linux, there are limitations or security that must be unlocked before testing, such as __open file__, __tcp_fin_timeout__, __tcp_tw_recycle__ and __tcp_tw_reuse__
  
  - Open file , 
  
    check number of open files by using `ulimit -n`
    
    
    __setting open files__
    
    ```bash
    ulimit -n 65536
    ```
    
    __Setting open files permanently__
    
    open `/etc/security/limits.conf` add add the below config.
    
    ```vim
    *               soft    nofile           65536
    *               hard    nofile           65536
    ```
    
  - tcp configuration
    
    __checking config__
    
    ```shell
    cat /etc/sysctl.conf |grep "net.ipv4.tcp_fin_timeout"
    cat /etc/sysctl.conf |grep "net.ipv4.tcp_tw_recycle"
    cat /etc/sysctl.conf |grep "net.ipv4.tcp_tw_reuse"
    ```
    
    __setting tcp configuration__
    
    ```
    echo 5 > /proc/sys/net/ipv4/tcp_fin_timeout
    echo 1 > /proc/sys/net/ipv4/tcp_tw_recycle
    echo 1 > /proc/sys/net/ipv4/tcp_tw_reuse
    ```
    
    > Note: Not recommend for server side

----

## Installation

  - Prerequisite for building __Tsunami__

    ```bash
    apt install make
    apt install golang-go
    ```
    
  - source code
    check out source from github.com
    ```bash
    git clone https://github.com/pepsi7959/tsunami.git
    ```
  
  - build
    ```bash
    cd clients && make
    ```
  
  - run
    ```bash
    /.tsunami --url [url]
    ```
   
----

## Features
  - **Realtime Monitoing**, There monitoring channel to monitor real-time statistics.
  - **Stand Alone**, Use only single binary.
  - **master node**, The master will control all of the workers.
  - **Independently scaling workers**, The workers, which are cloud sources, will be scaling independently.
  - **Support various protocols**, The protocols includes http/https and ldap.
