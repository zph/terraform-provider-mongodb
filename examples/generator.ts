import * as template from 'npm:nunjucks';
import { snakeCase, kebabCase } from 'npm:change-case';

// TODO: make sure throw works, it's not currently
// there are many issues in repo about this :-/
// Because it's unreliable we'll trust typescript typing instead
const env = template.configure('.', {throwOnUndefined: true})

env.addFilter('to_resource', function (str: string) {
  if(!str) throw new Error('to_resource: str is empty');
  return snakeCase(str.toLowerCase());
});

env.addFilter('kebab_case', function (str: string) {
  if(!str) throw new Error('kebab_case: str is empty');
  return kebabCase(str.toLowerCase());
});

// deno-lint-ignore no-explicit-any
function render(tmpl: string, args: {[key: string]: any}) {
  return env.render(tmpl, args)
}

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
  userAdminPassword: '<PASSWORD>',
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
  Exporter = 'mongo-exporter',
  Failover = 'failover',
}

const users: User[] = [
  {
    name: 'admin2',
    password: '<PASSWORD>',
    type: Role.Administrator,
  },
  {
    name: 'user2',
    password: '<PASSWORD>',
    type: Role.Administrator,
  },
  {
    name: 'user3',
    password: '<PASSWORD>',
    type: Role.Administrator,
  },
  {
    name: 'user4',
    password: '<PASSWORD>',
    type: Role.Administrator,
  },
  {
    name: 'user5',
    password: '<PASSWORD>',
    type: Role.Administrator,
  },
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
const servers = {
  'mongos': {
    host: 'localhost',
    port: 27017,
    userAdmin: 'admin',
    userAdminPassword: 'admin',
  },
  'shard-01': {
    host: 'localhost',
    port: 27018,
    userAdmin: 'admin',
    userAdminPassword: 'admin',
  },
  'shard-02': {
    host: 'localhost',
    port: 27021,
    userAdmin: 'admin',
    userAdminPassword: 'admin',
  },
  'shard-03': {
    host: 'localhost',
    port: 27024,
    userAdmin: 'admin',
    userAdminPassword: 'admin',
  },
}

const main = () => {
  const args: {nodes: Node[], users: User[]} = {
    nodes: Object.entries(servers).map(([name, opts]) => Server(name, opts)),
    users
  }
  const hcl = render('main.tf.j2', args)
  console.log(hcl)
}

main()
