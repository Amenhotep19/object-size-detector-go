package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const (
	// TIMEOUT is MQTT publish/subscribe timeout
	TIMEOUT = 1 * time.Second
	// QOS is Quality Of Service
	QOS = 1
)

// MQTTClient is MQTT client
type MQTTClient struct {
	// MQTT.Client implements MQTT client
	client MQTT.Client
}

// MQTTNewTLSConfig creates MQTT TLS configuration and returns it
// It returns error if it can't read TLS certificate files in provided paths.
func MQTTNewTLSConfig(crtPath, keyPath string, skipVerify bool) (*tls.Config, error) {
	// Import trusted certificates from CAfile.pem.
	// Alternatively, manually add CA certificates to
	// default openssl CA bundle.
	certpool := x509.NewCertPool()
	pemCerts, err := ioutil.ReadFile("samplecerts/CAfile.pem")
	if err == nil {
		certpool.AppendCertsFromPEM(pemCerts)
	}

	// Import client certificate/key pair
	cert, err := tls.LoadX509KeyPair(crtPath, keyPath)
	if err != nil {
		return nil, err
	}

	// Create tls.Config with desired tls properties
	return &tls.Config{
		// RootCAs = certs used to verify server cert.
		RootCAs: certpool,
		// ClientAuth = whether to request cert from server.
		// Since the server is set up for SSL, this happens
		// anyways.
		ClientAuth: tls.NoClientCert,
		// ClientCAs = certs used to validate client cert.
		ClientCAs: nil,
		// InsecureSkipVerify = verify that cert contents
		// match server. IP matches what is in cert etc.
		InsecureSkipVerify: skipVerify,
		// Certificates = list of certs client sends to server.
		Certificates: []tls.Certificate{cert},
	}, nil
}

// MQTTClientOptions creates new MQTT client options and returns it
// It reads the following environment variables to populate the options:
// MQTT_SERVER: URI address of MQTT server; required parameter
// MQTT_CLIENT_ID: MQTT client ID; required parameter
// MQTT_USERNAME: MQTT username; not required
// MQTT_PASSWORD: MQTT password for MQTT_USERNAME; not required
// MQTT_CERT: SSL certificate; not required
// MQTT_CERT_KEY: SSL certificate private key; not required
// MQTT_CA_ROOT: SSL CA root certificate; not required
// MQTT_TLS_SKIP_VERIFY: SSL TLS verification; not required
// It returns error if either MQTT server was not specified or if
// the MQTT client ID is missing in the client configuration options.
func MQTTClientOptions() (*MQTT.ClientOptions, error) {
	// read config options from environment variables
	server := os.Getenv("MQTT_SERVER")
	clientID := os.Getenv("MQTT_CLIENT_ID")
	username := os.Getenv("MQTT_USERNAME")
	password := os.Getenv("MQTT_PASSWORD")
	tlsCert := os.Getenv("MQTT_CERT")
	tlsKey := os.Getenv("MQTT_CERT_KEY")
	tlsCA := os.Getenv("MQTT_CA_ROOT")
	tlsSkipVerify := os.Getenv("MQTT_TLS_SKIP_VERIFY")

	if server == "" {
		return nil, fmt.Errorf("MQTT server is empty")
	}

	if clientID == "" {
		return nil, fmt.Errorf("MQTT clientID is empty")
	}

	opts := MQTT.NewClientOptions()
	opts.AddBroker(server)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(20 * time.Second)
	opts.CleanSession = true
	opts.SetPingTimeout(1 * time.Second)
	opts.SetDefaultPublishHandler(msgHandler)

	if username != "" && password != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}

	var skipVerify bool
	if tlsSkipVerify != "" {
		skipVerify = true
	}

	if tlsCert != "" && tlsKey != "" && tlsCA != "" {
		tlsConfig, err := MQTTNewTLSConfig(tlsCert, tlsKey, skipVerify)
		if err != nil {
			return nil, fmt.Errorf("Invalid TLS configuration: %s", err)
		}
		opts.SetTLSConfig(tlsConfig)
	}

	return opts, nil
}

// MQTTConnect attempts to connect to MQTT server and returns MQTT client
// It returns error if it fails to connect to the MQTT server.
func MQTTConnect(opts *MQTT.ClientOptions) (*MQTTClient, error) {
	c := MQTT.NewClient(opts)

	if token := c.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &MQTTClient{
		client: c,
	}, nil
}

// Publish publishes message to topic
// It returns MQTT connection Token
func (c *MQTTClient) Publish(topic, message string) (MQTT.Token, error) {
	token := c.client.Publish(topic, QOS, false, message)

	// wait for publish to finish
	if ok := token.WaitTimeout(TIMEOUT); ok && token.Error() != nil {
		return nil, token.Error()
	}

	return token, nil
}

// msgHandler for MQTT subscription for any desired control channel topic
func msgHandler(c MQTT.Client, msg MQTT.Message) {
	fmt.Printf("MQTT message received. Topic: %s Message: %s", msg.Topic(), msg.Payload())
}

// Subscribe subscribes to specified topic
// It returns MQTT connection Token
func (c *MQTTClient) Subscribe(topic string) (MQTT.Token, error) {
	token := c.client.Subscribe(topic, QOS, msgHandler)

	// wait for the subscription to finish
	if ok := token.WaitTimeout(TIMEOUT); ok && token.Error() != nil {
		return nil, token.Error()
	}

	return token, nil
}

// Disconnect closes the connection to MQTT broker, waiting for pending ms.
func (c *MQTTClient) Disconnect(pending uint) {
	c.client.Disconnect(pending)
}
