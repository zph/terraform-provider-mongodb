# Terraform Provider MongoDB

> Fork of [Kaginari/terraform-provider-mongodb](https://github.com/Kaginari/terraform-provider-mongodb).
> Forked to make larger changes than could be contributed via pull request to the upstream project and to iterate quickly for my own use cases. The changes are intended for production maturity but at this point the project is
largely unvalidated beyond the tests seen here.

This repository is a Terraform MongoDB provider for [Terraform](https://www.terraform.io).

### Why no MongoDB Atlas support?

This provider targets self-hosted MongoDB. We don't support MongoDB Atlas because we don't believe in fear-based extortion as a software engineering business model. If you need Atlas support, MongoDB has their own provider — best of luck with that.

### Why no Amazon DocumentDB support?

DocumentDB shipped with a single-writer architecture for its first years of existence. We judge that decision harshly and don't support it here.

### CDKTN Construct Library

The [`cdktn/`](cdktn/) directory contains a Go construct library for generating Terraform JSON
configs for sharded MongoDB clusters. Instead of hand-writing provider aliases and resources for
every node, define your cluster topology in Go and synthesize deterministic Terraform JSON.

See [`cdktn/README.md`](cdktn/README.md) for usage examples and API documentation.

### Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.7.5
- [Go](https://golang.org/doc/install) >= 1.17

### Installation

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the `make install` command:

````bash
git clone https://github.com/zph/terraform-provider-mongodb
cd terraform-provider-mongodb
make install
````

### To test locally

**1.1: create mongo image  with ssl**


````bash
cd docker/docker-mongo-ssl
docker build -t mongo-local .
````
**1.2: create ssl for localhost**


*follow the instruction in this link*

https://ritesh-yadav.github.io/tech/getting-valid-ssl-certificate-for-localhost-from-letsencrypt/


````bash
nano /etc/hosts
127.0.0.1   kaginar.herokuapp.com   ### add this line
````


**1.3: start the docker-compose**
````bash
cd docker
docker-compose up -d
````
**1.4 : create admin user in mongo**

````bash
$ docker exec -it mongo -c mongo
> use admin
> db.createUser({ user: "root" , pwd: "root", roles: ["userAdminAnyDatabase", "dbAdminAnyDatabase", "readWriteAnyDatabase"]})
````
**2: Build the provider**

follow the [Installation](#Installation)

**3: Use the provider**

````bash
cd mongodb
make apply
````
