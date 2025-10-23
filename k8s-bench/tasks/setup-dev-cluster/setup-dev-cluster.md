You are a Kubernetes administrator setting up a development cluster for a team of 3 developers (alice, bob, and charlie).

Create a secure, multi-tenant development environment with the following requirements:

1. **Namespaces**: Create separate namespaces for each developer (dev-alice, dev-bob, dev-charlie) plus shared namespaces (dev-shared, staging, prod)

2. **RBAC Configuration**:
    - Each developer should have full access to their own namespace
    - Developers should have read-only access to the dev-shared namespace
    - Only cluster admins should access staging and prod
    - Create service accounts for each developer (alice-sa, bob-sa, charlie-sa) in their respective namespaces
    - Each developer's service account should have full access to their respective namespace and read-only access to the dev-shared namespace

3. **Resource Quotas**:
    - Each developer namespace: max 2 CPUs, 4Gi memory, 10 pods, 5 services
    - dev-shared namespace: max 4 CPUs, 8Gi memory, 20 pods, 10 services  
    - staging/prod: max 8 CPUs, 16Gi memory, 50 pods, 20 services

4. **Network Policies**:
    - Developers can only access their own namespace and dev-shared
    - Block cross-developer namespace communication
    - Allow all namespaces to access DNS and system services
    - staging and prod should be completely isolated from dev namespaces

5. **Default Deny Policies**: Implement default deny network policies for all namespaces except system namespaces

Ensure all configurations follow principle of least privilege and provide appropriate isolation between environments.
