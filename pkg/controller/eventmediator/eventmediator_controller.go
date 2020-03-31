package eventmediator

import (
	"context"

    routev1 "github.com/openshift/api/route/v1"
	eventsv1alpha1 "github.com/kabanero-io/events-operator/pkg/apis/events/v1alpha1"
	"github.com/kabanero-io/events-operator/pkg/eventenv"
	"github.com/kabanero-io/events-operator/pkg/eventcel"
	corev1 "k8s.io/api/core/v1"
    appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
    "k8s.io/apimachinery/pkg/util/intstr"
    logf "sigs.k8s.io/controller-runtime/pkg/log"
    "github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
    "github.com/operator-framework/operator-sdk/pkg/k8sutil"
    "sigs.k8s.io/controller-runtime/pkg/predicate"
    "sigs.k8s.io/controller-runtime/pkg/event"

    "k8s.io/klog"
    // "os"
    "net/url"
    "net/http"
    "bytes"
    "time"
     "crypto/tls"
    "strings"
    "fmt"
    //"strconv"
)

var log = logf.Log.WithName("controller_eventmediator")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new EventMediator Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileEventMediator{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("eventmediator-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}


    controllerPredicate := predicate.Funcs{
        UpdateFunc: func(e event.UpdateEvent) bool {
            // Ignore status updates
            return e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration() 
        },
        CreateFunc: func(e event.CreateEvent) bool {
            return true
        },
        DeleteFunc: func(e event.DeleteEvent) bool {
            return true
        },
        GenericFunc: func(e event.GenericEvent) bool {
            return true
        },
    }

	// Watch for changes to primary resource EventMediator
	err = c.Watch(&source.Kind{Type: &eventsv1alpha1.EventMediator{}}, &handler.EnqueueRequestForObject{}, controllerPredicate)
	if err != nil {
		return err
	}

    // Watch for deployments
    if eventenv.GetEventEnv().IsOperator {
        err = c.Watch(
        &source.Kind{Type: &appsv1.Deployment{}},
        &handler.EnqueueRequestForOwner{
            IsController: true,
            OwnerType:    &eventsv1alpha1.EventMediator{}},
        )
	    if err != nil {
		    return err
        }

        err = c.Watch(
        &source.Kind{Type: &corev1.Service{}},
        &handler.EnqueueRequestForOwner{
            IsController: true,
            OwnerType:    &eventsv1alpha1.EventMediator{}},
        )
	    if err != nil {
		    return err
        }

        err = c.Watch(
        &source.Kind{Type: &routev1.Route{}},
        &handler.EnqueueRequestForOwner{
            IsController: true,
            OwnerType:    &eventsv1alpha1.EventMediator{}},
        )
	    if err != nil {
		    return err
        }
    }

	return nil
}

// blank assignment to verify that ReconcileEventMediator implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileEventMediator{}

// ReconcileEventMediator reconciles a EventMediator object
type ReconcileEventMediator struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a EventMediator object and makes changes based on the state read
// and what is in the EventMediator.Spec
// (user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileEventMediator) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling EventMediator")

	// Fetch the EventMediator instance
	instance := &eventsv1alpha1.EventMediator{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

    env := eventenv.GetEventEnv()
    if env.IsOperator {
        result, err := r.reconcileOperator(request, instance, reqLogger) 
        return result, err
    } else {
        /* plain controller for one mediator */
        if instance.ObjectMeta.Name ==  env.MediatorName {
            /*  We should handle this */
            env := eventenv.GetEventEnv()
            env.EventMgr.AddEventMediator(instance)

            if instance.Spec.CreateListener {
                port :=  getListenerPort(instance)
                key := eventsv1alpha1.MediatorHashKey(instance)
                err = env.ListenerMgr.NewListenerTLS(env, key, processMessage, eventenv.ListenerOptions{
                    Port: port,
                })
                if err != nil {
                     return reconcile.Result{}, err
                }
            }
        }
    }

	return reconcile.Result{}, nil
}

/* Reconcile deployment for an operator */
func (r *ReconcileEventMediator) reconcileOperator(request reconcile.Request, mediator *eventsv1alpha1.EventMediator, reqLogger logr.Logger) (reconcile.Result, error) {
    reqLogger.Info("In reconcileOperator")
    result, err := r.reconcileDeployment(request, mediator, reqLogger)
    if err != nil {
        reqLogger.Error(err, "error from reconcileDeployment")
        return result, err
    }
    result, err = r.reconcileService(request, mediator, reqLogger)
    if err != nil {
        reqLogger.Error(err, "error from reconcileService")
        return result, err
    }

    result, err = r.reconcileRoute(request, mediator, reqLogger)
    if err != nil {
        reqLogger.Error(err, "error from reconcileRoute")
        return result, err
    }
	return reconcile.Result{}, nil
}

/* Return service account name and image name of current operator pod */
func getPodInfo(client client.Client, namespace string) (string, string, error) {
    var imageName string
    pod, err := k8sutil.GetPod(context.TODO(), client, namespace)
    if err != nil {
        return "", "", err
    } else {
         serviceAccountName := pod.Spec.ServiceAccountName
         imageName = pod.Spec.Containers[0].Image
         return serviceAccountName, imageName, nil
    }
}

/* reconcile Operator */
func (r *ReconcileEventMediator) reconcileDeployment(request reconcile.Request, instance *eventsv1alpha1.EventMediator,  reqLogger logr.Logger) (reconcile.Result, error) {
    reqLogger.Info("In reconcileDeployment")
    /* Check if the deployment already exists, if not create a new one */
    deployment := &appsv1.Deployment{}
    err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
    if err != nil && errors.IsNotFound(err) {
        // Define a new deployment
        serviceAccountName, imageName, err := getPodInfo(r.client, instance.Namespace)
        dep := r.deploymentForEventMediator(instance, serviceAccountName, imageName)
        reqLogger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
        err = r.client.Create(context.TODO(), dep)
        if err != nil {
            reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
            return reconcile.Result{}, err
        }
        // Deployment created successfully - return and requeue
        return reconcile.Result{}, nil
    } else if err != nil {
        reqLogger.Error(err, "Failed to get Deployment")
        return reconcile.Result{}, err
    }

    /* Check if deployment should be changed */
    if portChangedForDeployment(deployment, instance)  {
        deployment.Spec.Template.Spec.Containers[0].Ports = generateDeploymentPorts(instance)
        err = r.client.Update(context.TODO(), deployment)
        if err != nil {
           reqLogger.Error(err, "Failed to update Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
            return reconcile.Result{}, err
         }
        // Spec updated - return and requeue
        return reconcile.Result{}, nil
    }

    return reconcile.Result{}, nil
}

/* reconcile Operator */
func (r *ReconcileEventMediator) reconcileService(request reconcile.Request, instance *eventsv1alpha1.EventMediator, reqLogger logr.Logger) (reconcile.Result, error) {
    reqLogger.Info("In reconcileService")
    service := &corev1.Service{}
    err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, service)
    if err != nil && errors.IsNotFound(err) {
        // Define a new service
        if instance.Spec.CreateListener {
            service = r.serviceForEventMediator(instance, reqLogger)
            reqLogger.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
            err = r.client.Create(context.TODO(), service)
            if err != nil {
                reqLogger.Error(err, "Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
                return reconcile.Result{}, err
            }
            // Service created successfully - return and requeue
            return reconcile.Result{}, nil
        } else {
            return reconcile.Result{}, nil
        }
    } else if err != nil {
        reqLogger.Error(err, "Failed to get Service")
        return reconcile.Result{}, err
    }

    if !instance.Spec.CreateListener {
         /* delete service. */
        err = r.client.Delete(context.Background(), service)
        if err != nil {
           reqLogger.Error(err, "Failed to delete service", "Service.Namespace", instance.Namespace, "Service.Name", instance.Name)
            return  reconcile.Result{}, nil
        }
        reqLogger.Info("Deleted service", "Service.Namespace", instance.Namespace, "Service.Name", instance.Name)
        return  reconcile.Result{}, nil
    }

    /* Check if service should be changed */
    if portChangedForService(service, instance)  {
        service.Spec.Ports = generateServicePorts(instance, reqLogger)
        err = r.client.Update(context.TODO(), service)
        if err != nil {
           reqLogger.Error(err, "Failed to update Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
            return reconcile.Result{}, err
         }
        // Spec updated - return and requeue
        return reconcile.Result{}, nil
    }
    return reconcile.Result{}, nil
}

/* Return true if the ports in a Deployment have changed */
func portChangedForDeployment(deployment *appsv1.Deployment, mediator *eventsv1alpha1.EventMediator) bool {

    ports := deployment.Spec.Template.Spec.Containers[0].Ports

    check := make(map[int32] int32)
    for _, portInfo := range ports {
        check[portInfo.ContainerPort] = portInfo.ContainerPort
    }

    numMediatorPorts := 0
    port := getListenerPort(mediator)
    numMediatorPorts++
    if   _, exists:= check[port]; ! exists {
        return true
    }
    if len(ports) != numMediatorPorts {
         return true
    }

    return false
}

func generateDeploymentPorts(mediator *eventsv1alpha1.EventMediator) []corev1.ContainerPort {
    var ports []corev1.ContainerPort = make([]corev1.ContainerPort, 0);
    port := int32(getListenerPort(mediator))
    ports = append(ports, corev1.ContainerPort {
           ContainerPort:  port,
           Name:          "port",
      } )
    return ports
}

// Return a deployment object
func (r *ReconcileEventMediator) deploymentForEventMediator(mediator *eventsv1alpha1.EventMediator, operatorServiceAccount string, imageName string) *appsv1.Deployment {

    ls := labelsForEventMediator(mediator.Name)
    var replicas int32 = 1
    // eventEnv := eventenv.GetEventEnv()
    env  := []corev1.EnvVar {
        {
             Name: eventenv.MEDIATOR_NAME_KEY,
             Value: mediator.Name,
        }, 
        {
             Name: "POD_NAME",
             ValueFrom:  &corev1.EnvVarSource {
                  FieldRef: &corev1.ObjectFieldSelector {
                       APIVersion: "v1",
                       FieldPath: "metadata.name",
                  },
             },
        },
        {
             Name: "WATCH_NAMESPACE",
             Value: mediator.Namespace,
        },
    }
    ports := generateDeploymentPorts(mediator)

    dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mediator.Name,
            Namespace: mediator.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: &replicas,
            Selector: &metav1.LabelSelector{
                MatchLabels: ls,
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: ls,
                },
                Spec: corev1.PodSpec{
                    ServiceAccountName: operatorServiceAccount,
                    Containers: []corev1.Container{
                      {
                        Image:   imageName,
                        Name:    "evnetmediator",
                        Command: []string{"entrypoint"},
                        Ports: ports,
                        Env: env,
                        VolumeMounts: []corev1.VolumeMount {
                             {
                             Name: "listener-certificates",
                             ReadOnly: true,
                             MountPath: "/etc/tls",
                             },
                        },
                    }},
                    Volumes: []corev1.Volume {
                      {
                          Name: "listener-certificates",
                          VolumeSource: corev1.VolumeSource {
                              Secret: &corev1.SecretVolumeSource{
                                  SecretName: mediator.Name,
                              },
                          },
                      },
                    },
                },
            },
        },
    }

    // Set owner and controller
    controllerutil.SetControllerReference(mediator, dep, r.scheme)
    return dep
}

func labelsForEventMediator(name string) map[string]string {
    return map[string]string{"app": name, "eventmediator_cr": name}
}

/* Get port in listener config. If port == 0, return default port. */
func getListenerPort(mediator *eventsv1alpha1.EventMediator) int32 {
    port := int32(0)
    if mediator != nil {
        port = mediator.Spec.ListenerPort
        if port == int32(0) {
            port = eventsv1alpha1.DEFAULT_HTTPS_PORT
        }
    }
    return port
}

func generateServicePorts(mediator *eventsv1alpha1.EventMediator, reqLogger logr.Logger) []corev1.ServicePort {
    ports := make([]corev1.ServicePort, 0)
    port := getListenerPort(mediator)
    ports = append(ports, corev1.ServicePort {
           Port:  int32(443),
           TargetPort: intstr.IntOrString { IntVal: port } ,
       } )
    return ports
}

// Return a Service object
func (r *ReconcileEventMediator) serviceForEventMediator(mediator *eventsv1alpha1.EventMediator, reqLogger logr.Logger) *corev1.Service {
    ls := labelsForEventMediator(mediator.Name)
    servicePorts := generateServicePorts(mediator, reqLogger)

    reqLogger.Info(fmt.Sprintf( "mediator: %v, ports: %v", mediator, servicePorts))

    service := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mediator.Name,
            Namespace: mediator.Namespace,
            Annotations: map[string]string {
                 "service.beta.openshift.io/serving-cert-secret-name": mediator.Name,
            },
        },
        Spec: corev1.ServiceSpec {
            Ports: servicePorts,
            Selector: ls,
            Type: corev1. ServiceTypeClusterIP,
        },
    }

    // Set owner and controller
    controllerutil.SetControllerReference(mediator, service, r.scheme)
    return service
}

/* Return true if the ports in a Service have changed */
func portChangedForService(service *corev1.Service, mediator *eventsv1alpha1.EventMediator) bool {

    ports := service.Spec.Ports

    check := make(map[int32] int32)
    for _, portInfo := range ports {
        check[portInfo.Port] = portInfo.Port
    }

   port := getListenerPort(mediator)
   numMediatorPorts:= 1
   if   _, exists:= check[port]; ! exists {
       return true
   }
    if len(ports) != numMediatorPorts {
         return true
    }
    return false
}


func processMessage(env *eventenv.EventEnv, message map[string]interface{}, key string, url *url.URL) error {
    klog.Infof("In processMessage message: %v, key: %v, url: %v, url path %v", message, key, url, url.Path)
    path := url.Path
    if strings.HasPrefix(path, "/") {
        path = path[1:]
    }

    mediator := env.EventMgr.GetMediator(key)
    if mediator == nil {
        klog.Info("No meditor found")
         // not for us
         return nil
    }
    if  mediator.Spec.Mediations == nil {
        klog.Info("No mediation within mediator")
         return nil
    }

    for _, mediationsImpl := range *mediator.Spec.Mediations {
         if  mediationsImpl.Mediation != nil {
              eventMediationImpl := mediationsImpl.Mediation
              if eventMediationImpl.Name == path {
                  /* process the message */
                  klog.Infof("Processing mediation %v", path)
                  processor := eventcel.NewProcessor(generateEventFunctionLookupHandler(mediator),generateSendEventHandler(env, mediator, path) )
                  _, err := processor.ProcessMessage(message, eventMediationImpl)
                  if err != nil {
                      klog.Errorf("Error processing mediation %v, error: %v", path, err)
                  }
                  return err
               }
         }
    }

    klog.Info("No matching mediation")
    return nil
}



func  generateEventFunctionLookupHandler (mediator *eventsv1alpha1.EventMediator) eventcel.GetEventFunctionHandler {
    return func(name string) *eventsv1alpha1.EventFunctionImpl {
        if mediator.Spec.Mediations == nil {
             return nil
         }

        for _, mediationsImpl := range *mediator.Spec.Mediations {
            if  mediationsImpl.Function != nil && mediationsImpl.Function.Name == name {
                return mediationsImpl.Function
             }
         }
         /* not found */
         return nil
    }
}

func generateSendEventHandler(env *eventenv.EventEnv, mediator *eventsv1alpha1.EventMediator, mediationName string) func(dest string, buf []byte, header map[string][]string) error {

    return func(destination string, buf[]byte, header map[string][]string) error {
        connectionsMgr  := env.ConnectionsMgr
        endpoint := &eventsv1alpha1.EventSourceEndpoint {
             Mediator: &eventsv1alpha1.EventMediatorSourceEndpoint {
                 Name: mediator.ObjectMeta.Name,
                 Mediation: mediationName,
                 Destination:  destination,
             },
         }
         destinations := connectionsMgr.LookupDestinationEndpoints(endpoint)
         for _, dest := range destinations {
             /* TODO: add configurable timeout */
             if dest.Https != nil {
                 for _, https := range *dest.Https {
                     timeout, _ := time.ParseDuration("5s")
                     klog.Infof("generateSendEventHandler: sending emssage to %v", https.Url)
                     err := sendMessage(https.Url, https.Insecure, timeout,  buf, header)
                     if err != nil  {
                         /* TODO: better way to handle errors */
                         klog.Errorf("generateSendEventHandler: error sending message: %v", err)
                         return err
                     }
                }
             }
         }
         return nil
    }
}

func sendMessage(url string, insecure bool, timeout time.Duration, payload []byte, header map[string][]string) error {
//    if klog.V(6) {
//        klog.Infof("restProvider: Sending %s", string(payload))
////    }

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
    if err != nil {
        return err
    }

    for key, arrayString := range header {
        for _, str := range arrayString {
            req.Header.Add(key, str)
        }
    }

    req.Header.Add("Content-Type", "application/json")
    tr := &http.Transport{}
    if insecure {
        tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
    }

    timeout = time.Duration(timeout* time.Second)
    client := &http.Client{
        Transport: tr,
        Timeout:   timeout,
    }

    resp, err := client.Do(req)
    if err != nil {
        return err
    }

    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("res_provider Send to %v failed with http status %v", url, resp.Status)
    }

    return nil
}


// Return a Service object
func (r *ReconcileEventMediator) routeForEventMediator(mediator *eventsv1alpha1.EventMediator, reqLogger logr.Logger) *routev1.Route {
    ls := labelsForEventMediator(mediator.Name)

    route := &routev1.Route {
        ObjectMeta: metav1.ObjectMeta{
            Name:      mediator.Name,
            Namespace: mediator.Namespace,
            Labels: ls,
        },
        Spec: routev1.RouteSpec {
            To: routev1.RouteTargetReference {
                 Kind: "Service",
                 Name: mediator.Name,
            },
            TLS: &routev1.TLSConfig {
                 Termination: routev1.TLSTerminationPassthrough,
            },
        },
    }

    // Set owner and controller
    controllerutil.SetControllerReference(mediator, route, r.scheme)
    return route
}

/* reconcile Operator */
func (r *ReconcileEventMediator) reconcileRoute(request reconcile.Request, instance *eventsv1alpha1.EventMediator, reqLogger logr.Logger) (reconcile.Result, error) {
    reqLogger.Info("In reconcileRoute")
    route := &routev1.Route{}
    err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, route)
    if err != nil && errors.IsNotFound(err) {
        if instance.Spec.CreateListener  && instance.Spec.CreateRoute {
            // Define a new Route
            route = r.routeForEventMediator(instance, reqLogger)
            reqLogger.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
            err = r.client.Create(context.TODO(), route)
            if err != nil {
                reqLogger.Error(err, "Failed to create new route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
                return reconcile.Result{}, err
            }
            // route successfully - return and requeue
            reqLogger.Info("New Route created. ", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
            return reconcile.Result{}, nil
          }
          return reconcile.Result{}, nil
    } else if err != nil {
        reqLogger.Error(err, "Failed to get Route")
        return reconcile.Result{}, err
    }

    if !instance.Spec.CreateListener  || !instance.Spec.CreateRoute {
        /* delete route */
       err  = r.client.Delete(context.Background(), route)
       if err != nil {
           reqLogger.Error(err, "Failed to delete route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
           return reconcile.Result{}, err
       } else {
           reqLogger.Info("Deleted Route ", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
           return reconcile.Result{}, nil
       }
    }
/*
    if portChangedForRoute(route, instance)  {
        route.Spec.Ports = generateRoutePorts(instance, reqLogger)
        err = r.client.Update(context.TODO(), route)
        if err != nil {
           reqLogger.Error(err, "Failed to update Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
            return reconcile.Result{}, err
         }
        // Spec updated - return and requeue
        return reconcile.Result{Requeue: true}, nil
    }
*/
    return reconcile.Result{}, nil
}
