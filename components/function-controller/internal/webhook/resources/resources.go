package resources

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctlrclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type WebhookConfig struct {
	Prefix           string
	Type             WebHookType
	CABundel         []byte
	ServiceName      string
	ServiceNamespace string
	Port             int32
}
type WebHookType string

const (
	MutatingWebhook   WebHookType = "Mutating"
	ValidatingWebHook WebHookType = "Validating"

	serverlessAPIGroup   = "serverless.kyma-project.io"
	serverlessAPIVersion = "v1alpha1"

	DefaultingWebhookName = "defaulting.webhook.serverless.kyma-project.io"
	ValidationWebhookName = "validation.webhook.serverless.kyma-project.io"
)

func EnsureWebhookConfigurationFor(ctx context.Context, client ctlrclient.Client, config WebhookConfig, wt WebHookType) error {
	if wt == MutatingWebhook {
		mwhc := &admissionregistrationv1.MutatingWebhookConfiguration{}
		if err := client.Get(ctx, types.NamespacedName{Name: DefaultingWebhookName}, mwhc); err != nil {
			if apiErrors.IsNotFound(err) {
				return client.Create(ctx, createMutatingWebhookConfiguration(config))
			}
			return errors.Wrapf(err, "failed to get defaulting MutatingWebhookConfiguration: %s", DefaultingWebhookName)
		}
		freshMwhc := createMutatingWebhookConfiguration(config)
		if !reflect.DeepEqual(freshMwhc.Webhooks, mwhc.Webhooks) {
			freshMwhc.SetResourceVersion(mwhc.GetResourceVersion())
			return client.Update(ctx, freshMwhc)
		}
		return nil
	}

	vwhc := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	if err := client.Get(ctx, types.NamespacedName{Name: ValidationWebhookName}, vwhc); err != nil {
		if apiErrors.IsNotFound(err) {
			return client.Create(ctx, createValidatingWebhookConfiguration(config))
		}
		return errors.Wrapf(err, "failed to get validation ValidatingWebhookConfiguration: %s", ValidationWebhookName)
	}
	freshVwhc := createValidatingWebhookConfiguration(config)
	if !reflect.DeepEqual(freshVwhc.Webhooks, vwhc.Webhooks) {
		freshVwhc.SetResourceVersion(vwhc.GetResourceVersion())
		return client.Update(ctx, freshVwhc)
	}
	return nil
}

func createMutatingWebhookConfiguration(config WebhookConfig) *admissionregistrationv1.MutatingWebhookConfiguration {
	failurePolicy := admissionregistrationv1.Fail
	matchPolicy := admissionregistrationv1.Exact
	reinvocationPolicy := admissionregistrationv1.NeverReinvocationPolicy
	scope := admissionregistrationv1.AllScopes
	sideEffects := admissionregistrationv1.SideEffectClassNone
	name := "defaulting.webhook.serverless.kyma-project.io"

	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: name,
				AdmissionReviewVersions: []string{
					"v1beta1",
					"v1",
				},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: config.CABundel,
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: config.ServiceNamespace,
						Name:      config.ServiceName,
						Path:      pointer.String("/defaulting"),
						Port:      pointer.Int32(443),
					},
				},
				FailurePolicy:      &failurePolicy,
				MatchPolicy:        &matchPolicy,
				ReinvocationPolicy: &reinvocationPolicy,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Rule: admissionregistrationv1.Rule{
							APIGroups: []string{
								serverlessAPIGroup,
							},
							APIVersions: []string{
								serverlessAPIVersion,
							},
							Resources: []string{"functions", "functions/status"},
							Scope:     &scope,
						},
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
						},
					},
				},
				SideEffects:    &sideEffects,
				TimeoutSeconds: pointer.Int32(30),
			},
		},
	}
}

func createValidatingWebhookConfiguration(config WebhookConfig) *admissionregistrationv1.ValidatingWebhookConfiguration {
	failurePolicy := admissionregistrationv1.Fail
	matchPolicy := admissionregistrationv1.Exact
	scope := admissionregistrationv1.AllScopes
	sideEffects := admissionregistrationv1.SideEffectClassNone
	name := "validation.webhook.serverless.kyma-project.io"

	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: name,
				AdmissionReviewVersions: []string{
					"v1beta1",
					"v1",
				},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: config.CABundel,
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: config.ServiceNamespace,
						Name:      config.ServiceName,
						Path:      pointer.String("/validation"),
						Port:      pointer.Int32(443),
					},
				},
				FailurePolicy: &failurePolicy,
				MatchPolicy:   &matchPolicy,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Rule: admissionregistrationv1.Rule{
							APIGroups: []string{
								serverlessAPIGroup,
							},
							APIVersions: []string{
								serverlessAPIVersion,
							},
							Resources: []string{"functions", "functions/status"},
							Scope:     &scope,
						},
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
						},
					},
					{
						Rule: admissionregistrationv1.Rule{
							APIGroups: []string{
								serverlessAPIGroup,
							},
							APIVersions: []string{
								serverlessAPIVersion,
							},
							Resources: []string{"gitrepositories", "gitrepositories/status"},
							Scope:     &scope,
						},
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
							admissionregistrationv1.Delete,
						},
					},
				},
				SideEffects:    &sideEffects,
				TimeoutSeconds: pointer.Int32(30),
			},
		},
	}
}
