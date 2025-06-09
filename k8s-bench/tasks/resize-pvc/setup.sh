#!/usr/bin/env bash
kubectl delete namespace storage --ignore-not-found
kubectl create namespace storage

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: storage-pvc
  namespace: storage
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard # This assumes the name of your StorageClass is "expandable-sc"
EOF

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: storage-pod
  namespace: storage
spec:
  volumes:
    - name: my-storage
      persistentVolumeClaim:
        claimName: storage-pvc # This assumes the name of your PersistentVolumeClaim is "storage-pvc"
  containers:
    - name: storage-user
      image: nginx:alpine
      volumeMounts:
        - name: my-storage
          mountPath: /usr/share/nginx/html
      ports:
        - containerPort: 80
EOF