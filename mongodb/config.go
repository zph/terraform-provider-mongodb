package mongodb

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/net/proxy"
)

type ClientConfig struct {
	Host               string
	Port               string
	Username           string
	Password           string
	DB                 string
	Ssl                bool
	InsecureSkipVerify bool
	ReplicaSet         string
	RetryWrites        bool
	Certificate        string
	Direct             bool
	Proxy              string
}
type DbUser struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type Role struct {
	Role string `json:"role"`
	Db   string `json:"db"`
}

func (role Role) String() string {
	return fmt.Sprintf("{ role : %s , db : %s }", role.Role, role.Db)
}

type PrivilegeDto struct {
	Db         string   `json:"db,omitempty"`
	Collection string   `json:"collection,omitempty"`
	Cluster    bool     `json:"cluster,omitempty"`
	Actions    []string `json:"actions"`
}

type Privilege struct {
	Resource Resource `json:"resource"`
	Actions  []string `json:"actions"`
}
type SingleResultGetUser struct {
	Users []struct {
		Id    string `json:"_id"`
		User  string `json:"user"`
		Db    string `json:"db"`
		Roles []struct {
			Role string `json:"role"`
			Db   string `json:"db"`
		} `json:"roles"`
	} `json:"users"`
}
type SingleResultGetRole struct {
	Roles []struct {
		Role           string `json:"role"`
		Db             string `json:"db"`
		InheritedRoles []struct {
			Role string `json:"role"`
			Db   string `json:"db"`
		} `json:"inheritedRoles"`
		Privileges []struct {
			Resource struct {
				Db         string `json:"db"`
				Collection string `json:"collection"`
				Cluster    bool   `json:"cluster"`
			} `json:"resource"`
			Actions []string `json:"actions"`
		} `json:"privileges"`
	} `json:"roles"`
}

func addArgs(arguments string, newArg string) string {
	if arguments != "" {
		return arguments + "&" + newArg
	} else {
		return "/?" + newArg
	}

}

// LOG-001: WHEN TF_LOG is set to DEBUG or TRACE, the provider SHALL log every
// MongoDB command name, target database, and request ID.
// LOG-002: WHEN a MongoDB command succeeds, the provider SHALL log the command
// name and duration.
// LOG-003: WHEN a MongoDB command fails, the provider SHALL log the command
// name, duration, and failure message at WARN level.
// LOG-004: WHEN authentication commands are sent, the provider SHALL NOT log
// credential values (driver default behavior — authenticate events have
// redacted command bodies).
func commandMonitor(ctx context.Context) *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(_ context.Context, e *event.CommandStartedEvent) {
			tflog.Debug(ctx, "mongo command started",
				map[string]interface{}{
					"command":    e.CommandName,
					"db":         e.DatabaseName,
					"request_id": e.RequestID,
					"body":       e.Command.String(),
				})
		},
		Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
			tflog.Debug(ctx, "mongo command succeeded",
				map[string]interface{}{
					"command":    e.CommandName,
					"duration":   e.Duration.String(),
					"request_id": e.RequestID,
				})
		},
		Failed: func(_ context.Context, e *event.CommandFailedEvent) {
			tflog.Warn(ctx, "mongo command failed",
				map[string]interface{}{
					"command":    e.CommandName,
					"duration":   e.Duration.String(),
					"failure":    e.Failure,
					"request_id": e.RequestID,
				})
		},
	}
}

// mongoClientOptions builds the shared *options.ClientOptions (URI, TLS, proxy)
// without setting authentication credentials.
func (c *ClientConfig) mongoClientOptions(ctx context.Context) (*options.ClientOptions, error) {
	var arguments = ""

	arguments = addArgs(arguments, "retrywrites="+strconv.FormatBool(c.RetryWrites))

	if c.Ssl {
		arguments = addArgs(arguments, "ssl=true")
	}

	if c.ReplicaSet != "" && !c.Direct {
		arguments = addArgs(arguments, "replicaSet="+c.ReplicaSet)
	}

	if c.Direct {
		arguments = addArgs(arguments, "connect=direct")
	}

	uri := "mongodb://" + c.Host + ":" + c.Port + arguments

	dialer, dialerErr := proxyDialer(c)
	if dialerErr != nil {
		return nil, dialerErr
	}

	opts := options.Client().ApplyURI(uri).SetDialer(dialer).SetMonitor(commandMonitor(ctx))

	if c.Certificate != "" {
		verify := c.InsecureSkipVerify
		tlsConfig, err := getTLSConfigWithAllServerCertificates([]byte(c.Certificate), verify)
		if err != nil {
			return nil, err
		}
		opts.SetTLSConfig(tlsConfig)
	}

	return opts, nil
}

// MongoClient creates a mongo.Client WITH authentication credentials.
func (c *ClientConfig) MongoClient(ctx context.Context) (*mongo.Client, error) {
	opts, err := c.mongoClientOptions(ctx)
	if err != nil {
		return nil, err
	}
	opts.SetAuth(options.Credential{
		AuthSource: c.DB, Username: c.Username, Password: c.Password,
	})
	return mongo.NewClient(opts)
}

// MongoClientNoAuth creates a mongo.Client WITHOUT authentication credentials.
// Used by the original_user resource to connect to a fresh MongoDB instance
// that has no users yet.
func (c *ClientConfig) MongoClientNoAuth(ctx context.Context) (*mongo.Client, error) {
	opts, err := c.mongoClientOptions(ctx)
	if err != nil {
		return nil, err
	}
	return mongo.NewClient(opts)
}

func getTLSConfigWithAllServerCertificates(ca []byte, verify bool) (*tls.Config, error) {
	/* As of version 1.2.1, the MongoDB Go Driver will only use the first CA server certificate found in sslcertificateauthorityfile.
	   The code below addresses this limitation by manually appending all server certificates found in sslcertificateauthorityfile
	   to a custom TLS configuration used during client creation. */

	tlsConfig := new(tls.Config)

	tlsConfig.InsecureSkipVerify = verify
	tlsConfig.RootCAs = x509.NewCertPool()
	ok := tlsConfig.RootCAs.AppendCertsFromPEM(ca)

	if !ok {
		return tlsConfig, errors.New("Failed parsing pem file")
	}

	return tlsConfig, nil
}

func (privilege Privilege) String() string {
	return fmt.Sprintf("{ resource : %s , actions : %s }", privilege.Resource, privilege.Actions)
}

type Resource struct {
	Db         string `json:"db,omitempty" bson:"db,omitempty"`
	Collection string `json:"collection,omitempty" bson:"collection,omitempty"`
	Cluster    bool   `json:"cluster,omitempty" bson:"cluster,omitempty"`
}

func (resource Resource) String() string {
	return fmt.Sprintf(" { db : %s , collection : %s }", resource.Db, resource.Collection)
}

func createUser(client *mongo.Client, user DbUser, roles []Role, database string) error {
	var result *mongo.SingleResult
	if len(roles) != 0 {
		result = client.Database(database).RunCommand(context.Background(), bson.D{{Key: "createUser", Value: user.Name},
			{Key: "pwd", Value: user.Password}, {Key: "roles", Value: roles}})
	} else {
		result = client.Database(database).RunCommand(context.Background(), bson.D{{Key: "createUser", Value: user.Name},
			{Key: "pwd", Value: user.Password}, {Key: "roles", Value: []bson.M{}}})
	}

	if result.Err() != nil {
		return result.Err()
	}
	return nil
}

func getUser(client *mongo.Client, username string, database string) (SingleResultGetUser, error) {
	var result *mongo.SingleResult
	result = client.Database(database).RunCommand(context.Background(), bson.D{{Key: "usersInfo", Value: bson.D{
		{Key: "user", Value: username},
		{Key: "db", Value: database},
	},
	}})
	var decodedResult SingleResultGetUser
	err := result.Decode(&decodedResult)
	if err != nil {
		return decodedResult, err
	}
	return decodedResult, nil
}

func getRole(client *mongo.Client, roleName string, database string) (SingleResultGetRole, error) {
	var result *mongo.SingleResult
	result = client.Database(database).RunCommand(context.Background(), bson.D{{Key: "rolesInfo", Value: bson.D{
		{Key: "role", Value: roleName},
		{Key: "db", Value: database},
	},
	},
		{Key: "showPrivileges", Value: true},
	})
	var decodedResult SingleResultGetRole
	err := result.Decode(&decodedResult)
	if err != nil {
		return decodedResult, err
	}
	return decodedResult, nil
}

func createRole(client *mongo.Client, role string, roles []Role, privilege []PrivilegeDto, database string) error {
	var privileges []Privilege
	var result *mongo.SingleResult
	for _, element := range privilege {
		var prv Privilege
		if element.Db != "" && element.Cluster {
			return fmt.Errorf("can't create a role that specifies both a db and a cluster=true: role %s", role)
		}

		prv.Resource = Resource{
			Db:         element.Db,
			Collection: element.Collection,
			Cluster:    element.Cluster,
		}
		prv.Actions = element.Actions
		privileges = append(privileges, prv)
	}
	var rolesValue any
	var privelegesValue any

	if len(roles) != 0 {
		rolesValue = roles
	} else {
		rolesValue = []bson.M{}
	}
	if len(privileges) != 0 {
		privelegesValue = privileges
	} else {
		privelegesValue = []bson.M{}
	}

	result = client.Database(database).RunCommand(context.Background(), bson.D{{Key: "createRole", Value: role},
		{Key: "privileges", Value: privelegesValue}, {Key: "roles", Value: rolesValue}})

	if result.Err() != nil {
		return result.Err()
	}
	return nil
}

func MongoClientInit(ctx context.Context, conf *MongoDatabaseConfiguration) (*mongo.Client, error) {
	client, err := conf.Config.MongoClient(ctx)
	if err != nil {
		return nil, err
	}
	return connectAndPing(ctx, client, conf.MaxConnLifetime)
}

// MongoClientInitNoAuth creates, connects, and pings a MongoDB client without
// authentication. Used for bootstrapping the original admin user.
func MongoClientInitNoAuth(ctx context.Context, conf *MongoDatabaseConfiguration) (*mongo.Client, error) {
	client, err := conf.Config.MongoClientNoAuth(ctx)
	if err != nil {
		return nil, err
	}
	return connectAndPing(ctx, client, conf.MaxConnLifetime)
}

func connectAndPing(ctx context.Context, client *mongo.Client, maxConnLifetime time.Duration) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, maxConnLifetime*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	return client, nil
}

func proxyDialer(c *ClientConfig) (options.ContextDialer, error) {
	proxyFromEnv := proxy.FromEnvironment().(options.ContextDialer)
	proxyFromProvider := c.Proxy

	if len(proxyFromProvider) > 0 {
		proxyURL, err := url.Parse(proxyFromProvider)
		if err != nil {
			return nil, err
		}
		proxyDialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}

		return proxyDialer.(options.ContextDialer), nil
	}

	return proxyFromEnv, nil
}
