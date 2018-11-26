# Building object size detector docker images in Microsoft Azure Cloud

This is a short step by step tutorial about how to build [docker](https://docker.com) image for `object-size-detector-go` application in [Microsoft Azure](https://azure.microsoft.com/) Cloud and make it available in [Azure Container Registry (ACR)](https://docs.microsoft.com/en-us/azure/container-registry/).

## Prerequisities

* Create a free Azure account by following the guide [here](https://azure.microsoft.com/en-us/free/)
* Install `azure-cli` tool by followin the guid [here](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest)

You should now be ready to proceed with the next steps

## Create Azure Container Registry

Before you can start building `docker` images you need to create a container registry where the built images will be stored and available for download. Before you can create an Azure Container Registry (ACR) you first need to create a [Resource Group](https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-overview#resource-groups) with which you will then associate your ACR.

Go ahead and login to your Azure account and create a new resource group by running the commands below:

```
az login
```

This will open a browser window and prompts you for you Azure password. Once you've authenticated you are ready to proceed further.

Pick a geographic location that suits your current geography. You can get a list of location as follows:

```
az account list-locations
```

For this tutorial we will work with assumption youre based in `westeurope`. Go ahead and create Azure Resource Group now:

```
az group create --name myResourceGroup --location westeurope
```

Now that you have created resource group, you can proceed by creating Azure Container Registry which will store all docker images and make them available for download. Note that you need to pick a **unique** name for your registry:

```
az acr create --resource-group myResourceGroup --name myOpenVinoGocv --sku Standard
```

With Azure Container Registry running on your account you can now proceed with building the application docker image.


## Build docker image and push to ACR

You are now ready to build a `docker` image for `object-size-detector-go` application and stored it in the ACR you had built earlier. We assume you have already cloned the `object-size-detector-go` git repository as per instructions in [README](./README.md).

First you need to log in to the ACR you built earlier:
```
az acr login --name myOpenVinoGocv
Login Succeeded
```

You can list all available ACRs in your Azure account:

```
az acr list --resource-group myResourceGroup --query "[].{acrLoginServer:loginServer}" --output table
AcrLoginServer
-------------------------
myopenvinogocv.azurecr.io
```

Before you are able to build and push new docker images into the ACR you need to allow access to it. There is a full documetation about Role Based Access Control using Azure AD which you can read about online. For the purpose of this guide we will grant ourselves **admin** privileges i.e. full read/write access:

```
az acr update --name myOpenVinoGocv --admin-enabled true
```

Now for the final part, we can build a docker image and automatically upload it to our ACR by running the command below:

```
az acr build --resource-group myResourceGroup --registry myOpenVinoGocv --image object-size-detector-go .
```

If everything went fine you should see the output similar to the one below:
```
The following dependencies were found:
- image:
    registry: myopenvinogocv.azurecr.io
    repository: object-size-detector-go
    tag: latest
    digest: sha256:fd1d337bf7384a8e33ed8a73a0948d02520ce0fada32ce24efac627c7de9de23
  runtime-dependency:
    registry: registry.hub.docker.com
    repository: library/ubuntu
    tag: "16.04"
    digest: sha256:e547ecaba7d078800c358082088e6cc710c3affd1b975601792ec701c80cdd39
  git: {}

2018/11/20 14:26:48 Successfully populated digests for step ID: build
2018/11/20 14:26:48 Step ID: push marked as successful (elapsed time in seconds: 131.494060)

Run ID: cb2 was successful after 7m3s
```

Now you are ready to run the example wherever you have `docker` cli available as follows. Note that the docker image tag contains a DNS name pointing to ACR printed in the output shown above under `registry` key:

```
docker run -it --rm myopenvinogocv.azurecr.io/object-size-detector-go -h
```

## Destrouy Azure environment

If you no longer need ACR you can easily remove all the resources by deleting particular resource group as follows:
```
az group delete --name myResourceGroup
```
