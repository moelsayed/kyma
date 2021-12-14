package scenarios

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"reflect"
	"strings"
	"time"

	serverlessv1alpha1 "github.com/kyma-project/kyma/components/function-controller/pkg/apis/serverless/v1alpha1"
	"github.com/kyma-project/kyma/tests/function-controller/pkg/function"
	"github.com/kyma-project/kyma/tests/function-controller/pkg/gitrepository"
	"github.com/kyma-project/kyma/tests/function-controller/pkg/poller"
	"github.com/kyma-project/kyma/tests/function-controller/pkg/secret"
	"github.com/kyma-project/kyma/tests/function-controller/pkg/shared"
	"github.com/kyma-project/kyma/tests/function-controller/pkg/step"
	"github.com/kyma-project/kyma/tests/function-controller/testsuite"

	"github.com/kyma-project/kyma/tests/function-controller/testsuite/gitops"
	"github.com/kyma-project/kyma/tests/function-controller/testsuite/teststep"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type testRepo struct {
	name             string
	provider         string
	url              string
	baseDir          string
	expectedResponse string
	reference        string
	secretKeys       []string
	runtime          serverlessv1alpha1.Runtime
	auth             *serverlessv1alpha1.RepositoryAuth
}

var testCases []testRepo = []testRepo{
	{
		name:             "github-func",
		provider:         "Github",
		url:              "git@github.com:moelsayed/pyhello.git",
		baseDir:          "/",
		reference:        "main",
		expectedResponse: "hello world",
		secretKeys:       []string{"GithubAuthPrivateKey"}, // config parameter name(s) in testsuite.Config
		runtime:          serverlessv1alpha1.Python39,
		auth: &serverlessv1alpha1.RepositoryAuth{
			Type:       serverlessv1alpha1.RepositoryAuthSSHKey,
			SecretName: "github-auth-secret",
		},
	},
	{
		name:             "azure-devops-func",
		provider:         "AzureDevOps",
		url:              "https://kyma-wookiee@dev.azure.com/kyma-wookiee/kyma-function/_git/kyma-function",
		baseDir:          "/code",
		reference:        "main",
		expectedResponse: "Hello Serverless",
		secretKeys: []string{ // config parameter name(s) in testsuite.Config
			"AzureDevOpsUsername",
			"AzureDevOpsPassword",
		},
		runtime: serverlessv1alpha1.Nodejs14,
		auth: &serverlessv1alpha1.RepositoryAuth{
			Type:       serverlessv1alpha1.RepositoryAuthBasic,
			SecretName: "azure-devops-auth-secret",
		},
	},
}

func GitAuthTestSteps(restConfig *rest.Config, cfg testsuite.Config, logf *logrus.Entry) (step.Step, error) {
	coreCli, err := typedcorev1.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "while creating k8s core client")
	}

	genericContainer, err := setupSharedContainer(restConfig, cfg, logf)
	if err != nil {
		return nil, errors.Wrapf(err, "while creating Shared Container")
	}
	poll := poller.Poller{
		MaxPollingTime:     cfg.MaxPollingTime,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		Log:                genericContainer.Log,
		DataKey:            testsuite.TestDataKey,
	}
	steps := []step.Step{}
	for _, testCase := range testCases {
		testSteps, err := gitAuthFunctionTestSteps(genericContainer, testCase, poll, cfg)
		if err != nil {
			return nil, errors.Wrapf(err, "while generated test case steps")
		}
		steps = append(steps, testSteps)
	}
	return step.NewSerialTestRunner(logf, "Test Git function authentication",
		teststep.NewNamespaceStep("Create test namespace", coreCli, genericContainer),
		step.NewParallelRunner(logf, "fn_tests",
			steps...)), nil
}

func setupSharedContainer(restConfig *rest.Config, cfg testsuite.Config, logf *logrus.Entry) (shared.Container, error) {
	currentDate := time.Now()
	cfg.Namespace = fmt.Sprintf("%s-%dh-%dm-%d", "test-serverless-gitauth", currentDate.Hour(), currentDate.Minute(), rand.Int())

	dynamicCli, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return shared.Container{}, errors.Wrapf(err, "while creating dynamic client")
	}

	return shared.Container{
		DynamicCli:  dynamicCli,
		Namespace:   cfg.Namespace,
		WaitTimeout: cfg.WaitTimeout,
		Verbose:     cfg.Verbose,
		Log:         logf,
	}, nil
}
func gitAuthFunctionTestSteps(genericContainer shared.Container, tr testRepo, poll poller.Poller, cfg testsuite.Config) (step.Step, error) {
	genericContainer.Log.Infof("Testing Git Function in namespace: %s", genericContainer.Namespace)

	secret := secret.NewSecret(tr.auth.SecretName, genericContainer)

	inClusterURL, err := url.Parse(fmt.Sprintf("http://%s.%s.svc.cluster.local", tr.name, genericContainer.Namespace))
	if err != nil {
		return nil, errors.Wrapf(err, "while parsing in-cluster URL")
	}

	// data, err := tr.authSecretDataFunc(tr)
	data, err := getSecretData(tr, cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "while generating secret data")
	}

	return step.NewSerialTestRunner(genericContainer.Log, fmt.Sprintf("%s Function auth test", tr.provider),
		teststep.CreateSecret(
			genericContainer.Log,
			secret,
			fmt.Sprintf("Create %s Auth Secret", tr.provider),
			data),
		teststep.NewCreateGitRepository(
			genericContainer.Log,
			gitrepository.New(fmt.Sprintf("%s-repo", tr.name), genericContainer),
			fmt.Sprintf("Create GitRepository for %s", tr.provider),
			gitops.AuthRepositorySpecWithURL(
				tr.url,
				tr.auth,
			)),
		teststep.CreateFunction(
			genericContainer.Log,
			function.NewFunction(tr.name, genericContainer),
			fmt.Sprintf("Create %s Function", tr.provider),
			gitops.GitopsFunction(
				fmt.Sprintf("%s-repo", tr.name),
				tr.baseDir,
				tr.reference,
				tr.runtime),
		),
		teststep.NewHTTPCheck(
			genericContainer.Log,
			"Git Function simple check through gateway",
			inClusterURL,
			poll, tr.expectedResponse)), nil
}

func getSecretData(tr testRepo, cfg testsuite.Config) (map[string]string, error) {
	c := reflect.ValueOf(&cfg)
	data := map[string]string{}
	for _, key := range tr.secretKeys {
		value := reflect.Indirect(c).FieldByName(key)
		if tr.auth.Type == serverlessv1alpha1.RepositoryAuthSSHKey {
			// this value will be base64 encoded since it's passed as an environment variable.
			// we have to decode it first before it's passed to the secret creator since it will re-encode it again.
			decoded, err := base64.StdEncoding.DecodeString(value.String())
			return map[string]string{"key": string(decoded)}, err
		}
		if strings.Contains(key, "Username") {
			data["username"] = value.String()
		}
		if strings.Contains(key, "Password") {
			data["password"] = value.String()
		}
	}
	if len(data) != 2 { // we expect a username and password
		return nil, errors.New("failed to read secrete data from testsuite config")
	}
	return data, nil
}
