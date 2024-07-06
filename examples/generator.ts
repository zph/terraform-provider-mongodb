#!/usr/bin/env -S deno run --allow-read="./"

import Templating, {snakeCase} from 'https://raw.githubusercontent.com/zph/terraform-generator/main/mod.ts'

type Node = {
  name: string,
  host: string,
  port: number,
  userAdmin: string,
  userAdminPassword: string,
}

const DefaultNode: Node = {
  name: 'default',
  host: 'localhost',
  port: 27017,
  userAdmin: 'admin',
  userAdminPassword: 'admin',
}

const Server = (name: string, opts: Partial<Node> = {}): Node => {
  const baseServer = {
    name: snakeCase(name),
  }

  return {
    ...DefaultNode,
    ...opts,
    ...baseServer,
  }
}

type User = {
  name: string,
  password: string,
  type: Role,
}

enum Role {
  Administrator = 'administrator',
  Staff = 'staff',
  Exporter = 'mongo-exporter',
  Failover = 'failover',
}

const users: User[] = [
  ...["admin2", "user2", "user3",
    "user4", "user5", "user6",
    "user7", "user8", "user9",
  ].flatMap(n => {
    return [
      {
        name: n,
        password: '<PASSWORD>',
        type: Role.Staff,
      },
      {
        name: `${n}-admin`,
        password: '<PASSWORD>',
        type: Role.Administrator,
      },
    ]
  }),
  {
    name: 'mongo_exporter',
    password: '<PASSWORD>',
    type: Role.Exporter,
  },
  {
    name: 'failover',
    password: 'fail',
    type: Role.Failover,
  },
]
const servers = [
    { name: 'mongos', port: 27017 },
    { name: 'shard-01', port: 27018 },
    { name: 'shard-02', port: 27021 },
    { name: 'shard-03', port: 27024 }
  ].map(({ name, port }) => Server(name, { port }))

const main = () => {
  const args: { nodes: Node[], users: User[] } = {
    nodes: servers,
    users
  }
  const template = new Templating('.')
  const hcl = template.render('main.tf.j2', args)
  console.log(hcl)
}

main()
