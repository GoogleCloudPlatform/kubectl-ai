#!/usr/bin/env bash
kubectl delete namespace homepage-ns --ignore-not-found
kubectl create namespace homepage-ns

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: homepage-pvc
  namespace: homepage-ns
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: sc 
  resources:
    requests:
      storage: 1Gi
EOF

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: homepage-pod
  namespace: homepage-ns
spec:
  containers:
    - name: nginx
      image: nginx:latest
      ports:
        - containerPort: 80
      volumeMounts:
        - name: my-persistent-storage
          mountPath: /usr/share/nginx/html
  volumes:
    - name: my-persistent-storage
      persistentVolumeClaim:
        claimName: homepage-pvc
EOF