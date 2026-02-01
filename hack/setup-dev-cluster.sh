#!/usr/bin/env bash
#
# setup-dev-cluster.sh - Create a local Kubernetes cluster for development
#
# Usage:
#   ./hack/setup-dev-cluster.sh [options]
#
# Options:
#   --driver <minikube|kind>   Cluster driver (default: minikube)
#   --skip-cert-manager        Skip cert-manager installation
#   --skip-prometheus          Skip prometheus-operator installation
#   --delete                   Delete existing cluster first
#   -h, --help                 Show this help message

set -euo pipefail

# Versions
CERT_MANAGER_VERSION="v1.19.2"
PROMETHEUS_OPERATOR_VERSION="v0.88.1"

# Defaults
DRIVER="minikube"
SKIP_CERT_MANAGER=false
SKIP_PROMETHEUS=false
DELETE_FIRST=false
CLUSTER_NAME="uptime-robot-dev"

# Colours
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Colour

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

usage() {
    sed -n '2,/^$/p' "$0" | grep '^#' | cut -c3-
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --driver)
            DRIVER="$2"
            shift 2
            ;;
        --skip-cert-manager)
            SKIP_CERT_MANAGER=true
            shift
            ;;
        --skip-prometheus)
            SKIP_PROMETHEUS=true
            shift
            ;;
        --delete)
            DELETE_FIRST=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            ;;
    esac
done

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Please install it first."
        exit 1
    fi
    
    case $DRIVER in
        minikube)
            if ! command -v minikube &> /dev/null; then
                log_error "minikube is not installed. Please install it first."
                log_info "Install: brew install minikube"
                exit 1
            fi
            ;;
        kind)
            if ! command -v kind &> /dev/null; then
                log_error "kind is not installed. Please install it first."
                log_info "Install: brew install kind"
                exit 1
            fi
            ;;
        *)
            log_error "Unknown driver: $DRIVER. Use 'minikube' or 'kind'."
            exit 1
            ;;
    esac
    
    log_info "Prerequisites check passed"
}

# Delete existing cluster
delete_cluster() {
    log_info "Deleting existing cluster..."
    
    case $DRIVER in
        minikube)
            minikube delete --profile "$CLUSTER_NAME" 2>/dev/null || true
            ;;
        kind)
            kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
            ;;
    esac
}

# Create cluster
create_cluster() {
    log_info "Creating $DRIVER cluster: $CLUSTER_NAME"
    
    case $DRIVER in
        minikube)
            if minikube status --profile "$CLUSTER_NAME" &>/dev/null; then
                log_warn "Cluster already exists. Use --delete to recreate."
                minikube profile "$CLUSTER_NAME"
                return 0
            fi
            minikube start \
                --profile "$CLUSTER_NAME" \
                --cpus 4 \
                --memory 8192 \
                --kubernetes-version stable \
                --driver docker
            ;;
        kind)
            if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
                log_warn "Cluster already exists. Use --delete to recreate."
                kubectl config use-context "kind-${CLUSTER_NAME}"
                return 0
            fi
            kind create cluster --name "$CLUSTER_NAME" --wait 60s
            ;;
    esac
    
    log_info "Cluster created successfully"
}

# Wait for cluster to be ready
wait_for_cluster() {
    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s
    log_info "Cluster is ready"
}

# Install cert-manager
install_cert_manager() {
    if [[ "$SKIP_CERT_MANAGER" == "true" ]]; then
        log_info "Skipping cert-manager installation"
        return 0
    fi
    
    log_info "Installing cert-manager ${CERT_MANAGER_VERSION}..."
    
    # Check if already installed
    if kubectl get crd certificates.cert-manager.io &>/dev/null; then
        log_warn "cert-manager CRDs already exist, skipping installation"
        return 0
    fi
    
    kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
    
    log_info "Waiting for cert-manager to be ready..."
    kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s
    
    log_info "cert-manager installed successfully"
}

# Install prometheus-operator
install_prometheus_operator() {
    if [[ "$SKIP_PROMETHEUS" == "true" ]]; then
        log_info "Skipping prometheus-operator installation"
        return 0
    fi
    
    log_info "Installing prometheus-operator ${PROMETHEUS_OPERATOR_VERSION}..."
    
    # Check if already installed
    if kubectl get crd prometheuses.monitoring.coreos.com &>/dev/null; then
        log_warn "prometheus-operator CRDs already exist, skipping installation"
        return 0
    fi
    
    kubectl apply --server-side -f "https://github.com/prometheus-operator/prometheus-operator/releases/download/${PROMETHEUS_OPERATOR_VERSION}/bundle.yaml"
    
    log_info "Waiting for prometheus-operator to be ready..."
    kubectl wait --for=condition=Available deployment/prometheus-operator -n default --timeout=120s
    
    log_info "prometheus-operator installed successfully"
}

# Install CRDs
install_crds() {
    log_info "Installing uptime-robot-operator CRDs..."
    
    if [[ -f "Makefile" ]]; then
        make install
    else
        log_warn "Makefile not found, skipping CRD installation"
    fi
}

# Print next steps
print_next_steps() {
    echo ""
    log_info "============================================="
    log_info "Development cluster setup complete!"
    log_info "============================================="
    echo ""
    echo "Next steps:"
    echo ""
    echo "  1. Run the operator locally:"
    echo "     make run"
    echo ""
    echo "  2. Or build and deploy to the cluster:"
    echo "     make docker-build IMG=uptime-robot-operator:dev"
    case $DRIVER in
        minikube)
            echo "     minikube image load uptime-robot-operator:dev --profile $CLUSTER_NAME"
            ;;
        kind)
            echo "     kind load docker-image uptime-robot-operator:dev --name $CLUSTER_NAME"
            ;;
    esac
    echo "     make deploy IMG=uptime-robot-operator:dev"
    echo ""
    echo "  3. Create a test monitor:"
    echo "     kubectl apply -f config/samples/"
    echo ""
    echo "  4. Run e2e tests:"
    echo "     make test-e2e"
    echo ""
    echo "  5. Delete the cluster when done:"
    case $DRIVER in
        minikube)
            echo "     minikube delete --profile $CLUSTER_NAME"
            ;;
        kind)
            echo "     kind delete cluster --name $CLUSTER_NAME"
            ;;
    esac
    echo ""
}

# Main
main() {
    check_prerequisites
    
    if [[ "$DELETE_FIRST" == "true" ]]; then
        delete_cluster
    fi
    
    create_cluster
    wait_for_cluster
    install_cert_manager
    install_prometheus_operator
    install_crds
    print_next_steps
}

main "$@"
