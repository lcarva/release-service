package loader

import (
	"context"
	"fmt"
	ecapiv1alpha1 "github.com/hacbs-contract/enterprise-contract-controller/api/v1alpha1"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/release-service/api/v1alpha1"
	"github.com/redhat-appstudio/release-service/tekton"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type ObjectLoader interface {
	GetActiveReleasePlanAdmission(ctx context.Context, cli client.Client, releasePlan *v1alpha1.ReleasePlan) (*v1alpha1.ReleasePlanAdmission, error)
	GetActiveReleasePlanAdmissionFromRelease(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*v1alpha1.ReleasePlanAdmission, error)
	GetApplication(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*applicationapiv1alpha1.Application, error)
	GetApplicationComponents(ctx context.Context, cli client.Client, application *applicationapiv1alpha1.Application) ([]applicationapiv1alpha1.Component, error)
	GetEnterpriseContractPolicy(ctx context.Context, cli client.Client, releaseStrategy *v1alpha1.ReleaseStrategy) (*ecapiv1alpha1.EnterpriseContractPolicy, error)
	GetEnvironment(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*applicationapiv1alpha1.Environment, error)
	GetRelease(ctx context.Context, cli client.Client, name, namespace string) (*v1alpha1.Release, error)
	GetReleasePipelineRun(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*v1beta1.PipelineRun, error)
	GetReleasePlan(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*v1alpha1.ReleasePlan, error)
	GetReleaseStrategy(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*v1alpha1.ReleaseStrategy, error)
	GetSnapshot(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*applicationapiv1alpha1.Snapshot, error)
	GetSnapshotEnvironmentBinding(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error)
	GetSnapshotEnvironmentBindingFromReleaseStatus(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error)
	GetSnapshotEnvironmentBindingResources(ctx context.Context, cli client.Client, release *v1alpha1.Release, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*SnapshotEnvironmentBindingResources, error)
}

type loader struct{}

func NewLoader() ObjectLoader {
	return &loader{}
}

// getObject loads an object from the cluster. This is a generic function that requires the object to be passed as an
// argument. The object is modified during the invocation.
func getObject(name, namespace string, cli client.Client, ctx context.Context, object client.Object) error {
	return cli.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, object)
}

// GetActiveReleasePlanAdmission returns the ReleasePlanAdmission targeted by the given ReleasePlan.
// Only ReleasePlanAdmissions with the 'auto-release' label set to true (or missing the label, which is
// treated the same as having the label and it being set to true) will be searched for. If a matching
// ReleasePlanAdmission is not found or the List operation fails, an error will be returned. If more than
// one matching ReleasePlanAdmission objects is found, an error will be returned.
func (l *loader) GetActiveReleasePlanAdmission(ctx context.Context, cli client.Client, releasePlan *v1alpha1.ReleasePlan) (*v1alpha1.ReleasePlanAdmission, error) {
	releasePlanAdmissions := &v1alpha1.ReleasePlanAdmissionList{}
	err := cli.List(ctx, releasePlanAdmissions,
		client.InNamespace(releasePlan.Spec.Target),
		client.MatchingFields{"spec.origin": releasePlan.Namespace})
	if err != nil {
		return nil, err
	}

	var activeReleasePlanAdmission *v1alpha1.ReleasePlanAdmission

	for i, releasePlanAdmission := range releasePlanAdmissions.Items {
		if releasePlanAdmission.Spec.Application != releasePlan.Spec.Application {
			continue
		}

		if activeReleasePlanAdmission != nil {
			return nil, fmt.Errorf("multiple ReleasePlanAdmissions found with the target (%+v) for application '%s'",
				releasePlan.Spec.Target, releasePlan.Spec.Application)
		}

		labelValue, found := releasePlanAdmission.GetLabels()[v1alpha1.AutoReleaseLabel]
		if found && labelValue == "false" {
			return nil, fmt.Errorf("found ReleasePlanAdmission '%s' with auto-release label set to false",
				releasePlanAdmission.Name)
		}
		activeReleasePlanAdmission = &releasePlanAdmissions.Items[i]
	}

	if activeReleasePlanAdmission == nil {
		return nil, fmt.Errorf("no ReleasePlanAdmission found in the target (%+v) for application '%s'",
			releasePlan.Spec.Target, releasePlan.Spec.Application)
	}

	return activeReleasePlanAdmission, nil
}

// GetActiveReleasePlanAdmissionFromRelease returns the ReleasePlanAdmission targeted by the ReleasePlan referenced by
// the given Release. Only ReleasePlanAdmissions with the 'auto-release' label set to true (or missing the label, which
// is treated the same as having the label and it being set to true) will be searched for. If a matching
// ReleasePlanAdmission is not found or the List operation fails, an error will be returned.
func (l *loader) GetActiveReleasePlanAdmissionFromRelease(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*v1alpha1.ReleasePlanAdmission, error) {
	releasePlan, err := l.GetReleasePlan(ctx, cli, release)
	if err != nil {
		return nil, err
	}

	return l.GetActiveReleasePlanAdmission(ctx, cli, releasePlan)
}

// GetApplication returns the Application referenced by the ReleasePlanAdmission. If the Application is not found or
// the Get operation fails, an error will be returned.
func (l *loader) GetApplication(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*applicationapiv1alpha1.Application, error) {
	application := &applicationapiv1alpha1.Application{}
	return application, getObject(releasePlanAdmission.Spec.Application, releasePlanAdmission.Namespace, cli, ctx, application)
}

// GetApplicationComponents returns a list of all the Components associated with the given Application.
func (l *loader) GetApplicationComponents(ctx context.Context, cli client.Client, application *applicationapiv1alpha1.Application) ([]applicationapiv1alpha1.Component, error) {
	applicationComponents := &applicationapiv1alpha1.ComponentList{}
	err := cli.List(ctx, applicationComponents,
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name})
	if err != nil {
		return nil, err
	}

	return applicationComponents.Items, nil
}

// GetEnterpriseContractPolicy returns the EnterpriseContractPolicy referenced by the given ReleaseStrategy. If the
// EnterpriseContractPolicy is not found or the Get operation fails, an error is returned.
func (l *loader) GetEnterpriseContractPolicy(ctx context.Context, cli client.Client, releaseStrategy *v1alpha1.ReleaseStrategy) (*ecapiv1alpha1.EnterpriseContractPolicy, error) {
	enterpriseContractPolicy := &ecapiv1alpha1.EnterpriseContractPolicy{}
	return enterpriseContractPolicy, getObject(releaseStrategy.Spec.Policy, releaseStrategy.Namespace, cli, ctx, enterpriseContractPolicy)
}

// GetEnvironment returns the Environment referenced by the given ReleasePlanAdmission. If the Environment is not found
// or the Get operation fails, an error will be returned.
func (l *loader) GetEnvironment(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*applicationapiv1alpha1.Environment, error) {
	environment := &applicationapiv1alpha1.Environment{}
	return environment, getObject(releasePlanAdmission.Spec.Environment, releasePlanAdmission.Namespace, cli, ctx, environment)
}

// GetRelease returns the Release with the given name and namespace. If the Release is not found or the Get operation
// fails, an error will be returned.
func (l *loader) GetRelease(ctx context.Context, cli client.Client, name, namespace string) (*v1alpha1.Release, error) {
	release := &v1alpha1.Release{}
	return release, getObject(name, namespace, cli, ctx, release)
}

// GetReleasePipelineRun returns the PipelineRun referenced by the given Release or nil if it's not found. In the case
// the List operation fails, an error will be returned.
func (l *loader) GetReleasePipelineRun(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*v1beta1.PipelineRun, error) {
	pipelineRuns := &v1beta1.PipelineRunList{}
	err := cli.List(ctx, pipelineRuns,
		client.Limit(1),
		client.MatchingLabels{
			tekton.ReleaseNameLabel:      release.Name,
			tekton.ReleaseNamespaceLabel: release.Namespace,
		})
	if err == nil && len(pipelineRuns.Items) > 0 {
		return &pipelineRuns.Items[0], nil
	}

	return nil, err
}

// GetReleasePlan returns the ReleasePlan referenced by the given Release. If the ReleasePlan is not found or
// the Get operation fails, an error will be returned.
func (l *loader) GetReleasePlan(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*v1alpha1.ReleasePlan, error) {
	releasePlan := &v1alpha1.ReleasePlan{}
	return releasePlan, getObject(release.Spec.ReleasePlan, release.Namespace, cli, ctx, releasePlan)
}

// GetReleaseStrategy returns the ReleaseStrategy referenced by the given ReleasePlanAdmission. If the ReleaseStrategy
// is not found or the Get operation fails, an error will be returned.
func (l *loader) GetReleaseStrategy(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*v1alpha1.ReleaseStrategy, error) {
	releaseStrategy := &v1alpha1.ReleaseStrategy{}
	return releaseStrategy, getObject(releasePlanAdmission.Spec.ReleaseStrategy, releasePlanAdmission.Namespace, cli, ctx, releaseStrategy)
}

// GetSnapshot returns the Snapshot referenced by the given Release. If the Snapshot is not found or the Get
// operation fails, an error is returned.
func (l *loader) GetSnapshot(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*applicationapiv1alpha1.Snapshot, error) {
	snapshot := &applicationapiv1alpha1.Snapshot{}
	return snapshot, getObject(release.Spec.Snapshot, release.Namespace, cli, ctx, snapshot)
}

// GetSnapshotEnvironmentBinding returns the SnapshotEnvironmentBinding associated with the given ReleasePlanAdmission.
// That association is defined by both the Environment and Application matching between the ReleasePlanAdmission and
// the SnapshotEnvironmentBinding. If the Get operation fails, an error will be returned.
func (l *loader) GetSnapshotEnvironmentBinding(ctx context.Context, cli client.Client, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error) {
	bindingList := &applicationapiv1alpha1.SnapshotEnvironmentBindingList{}
	err := cli.List(ctx, bindingList,
		client.InNamespace(releasePlanAdmission.Namespace),
		client.MatchingFields{"spec.environment": releasePlanAdmission.Spec.Environment})
	if err != nil {
		return nil, err
	}

	for _, binding := range bindingList.Items {
		if binding.Spec.Application == releasePlanAdmission.Spec.Application {
			return &binding, nil
		}
	}

	return nil, nil
}

// GetSnapshotEnvironmentBindingFromReleaseStatus returns the SnapshotEnvironmentBinding associated with the given Release.
// That association is defined by namespaced name stored in the Release's status.
func (l *loader) GetSnapshotEnvironmentBindingFromReleaseStatus(ctx context.Context, cli client.Client, release *v1alpha1.Release) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error) {
	binding := &applicationapiv1alpha1.SnapshotEnvironmentBinding{}
	bindingNamespacedName := strings.Split(release.Status.SnapshotEnvironmentBinding, string(types.Separator))
	if len(bindingNamespacedName) != 2 {
		return nil, fmt.Errorf("release doesn't contain a valid reference to an SnapshotEnvironmentBinding ('%s')",
			release.Status.SnapshotEnvironmentBinding)
	}

	err := cli.Get(ctx, types.NamespacedName{
		Namespace: bindingNamespacedName[0],
		Name:      bindingNamespacedName[1],
	}, binding)

	if err != nil {
		return nil, err
	}

	return binding, nil
}

// Composite functions

// SnapshotEnvironmentBindingResources contains the required resources for creating a SnapshotEnvironmentBinding.
type SnapshotEnvironmentBindingResources struct {
	Application           *applicationapiv1alpha1.Application
	ApplicationComponents []applicationapiv1alpha1.Component
	Environment           *applicationapiv1alpha1.Environment
	Snapshot              *applicationapiv1alpha1.Snapshot
}

// GetSnapshotEnvironmentBindingResources returns all the resources required to create a SnapshotEnvironmentBinding.
// If any of those resources cannot be retrieved from the cluster, an error will be returned.
func (l *loader) GetSnapshotEnvironmentBindingResources(ctx context.Context, cli client.Client, release *v1alpha1.Release, releasePlanAdmission *v1alpha1.ReleasePlanAdmission) (*SnapshotEnvironmentBindingResources, error) {
	resources := &SnapshotEnvironmentBindingResources{}

	application, err := l.GetApplication(ctx, cli, releasePlanAdmission)
	if err != nil {
		return resources, err
	}
	resources.Application = application

	applicationComponents, err := l.GetApplicationComponents(ctx, cli, application)
	if err != nil {
		return resources, err
	}
	resources.ApplicationComponents = applicationComponents

	environment, err := l.GetEnvironment(ctx, cli, releasePlanAdmission)
	if err != nil {
		return resources, err
	}
	resources.Environment = environment

	snapshot, err := l.GetSnapshot(ctx, cli, release)
	if err != nil {
		return resources, err
	}
	resources.Snapshot = snapshot

	return resources, nil
}
