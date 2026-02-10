#!/usr/bin/env bash
#
# setup-dev-cluster.sh - Create a local Kind cluster for development
#
# Usage:
#   ./hack/setup-dev-cluster.sh [options]
#
# Options:
#   --delete                   Delete existing cluster first
#   -h, --help                 Show this help message

set -euo pipefail

# Defaults
DELETE_FIRST=false
CLUSTER_NAME="kind"

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
    
    if ! command -v kind &> /dev/null; then
        log_error "kind is not installed. Please install it first."
        log_info "Install: brew install kind"
        exit 1
    fi
    
    log_info "Prerequisites check passed"
}

# Delete existing cluster
delete_cluster() {
    log_info "Deleting existing cluster..."
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
}

# Create cluster
create_cluster() {
    log_info "Creating Kind cluster: $CLUSTER_NAME"
    
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_warn "Cluster already exists. Use --delete to recreate."
        kubectl config use-context "kind-${CLUSTER_NAME}"
        return 0
    fi
    
    kind create cluster --name "$CLUSTER_NAME" --wait 60s
    log_info "Cluster created successfully"
}

# Wait for cluster to be ready
wait_for_cluster() {
    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s
    log_info "Cluster is ready"
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

# Install cert-manager (required for webhook TLS certificates)
install_cert_manager() {
    log_info "Installing pinned cert-manager version..."

    if [[ -f "Makefile" ]]; then
        make cert-manager-install
    else
        log_warn "Makefile not found, skipping cert-manager installation"
    fi
}

# Build and deploy operator
build_and_deploy_operator() {
    log_info "Building operator image..."
    
    if [[ ! -f "Makefile" ]]; then
        log_warn "Makefile not found, skipping operator build and deployment"
        return 0
    fi
    
    # Build the operator image
    make docker-build IMG=uptime-robot-operator:dev
    
    log_info "Loading operator image into cluster..."
    kind load docker-image uptime-robot-operator:dev --name "$CLUSTER_NAME"
    
    log_info "Deploying operator to cluster..."
    
    # Deploy the operator
    make deploy IMG=uptime-robot-operator:dev
    
    log_info "Operator deployed successfully"
}

# Print next steps
print_next_steps() {
    echo ""
    log_info "============================================="
    log_info "Development cluster setup complete!"
    log_info "============================================="
    echo ""
    echo "The operator has been built and deployed to the cluster."
    echo ""
    echo "Cluster name: $CLUSTER_NAME"
    echo "Kubectl context: kind-$CLUSTER_NAME"
    echo ""
    echo "Next steps:"
    echo ""
    echo "  1. Create a test monitor:"
    echo "     kubectl apply -f config/samples/"
    echo ""
    echo "  2. Check operator logs:"
    echo "     kubectl logs -n uptime-robot-operator-system deployment/uptime-robot-operator-controller-manager -f"
    echo ""
    echo "  3. Run basic e2e tests:"
    echo "     make test-e2e"
    echo ""
    echo "  4. Run full e2e tests (requires UPTIME_ROBOT_API_KEY):"
    echo "     export UPTIME_ROBOT_API_KEY=your-test-api-key"
    echo "     make test-e2e-real"
    echo ""
    echo "  5. Rebuild and redeploy after changes:"
    echo "     make docker-build IMG=uptime-robot-operator:dev"
    echo "     kind load docker-image uptime-robot-operator:dev --name $CLUSTER_NAME"
    echo "     kubectl rollout restart -n uptime-robot-operator-system deployment/uptime-robot-operator-controller-manager"
    echo ""
    echo "  6. Delete the cluster when done:"
    echo "     make dev-cluster-delete"
    echo ""
}

# Main
main() {
    check_prerequisites
    
    if [[ "$DELETE_FIRST" == "true" ]]; then
        delete_cluster
        log_info "Cluster deleted successfully"
        exit 0
    fi
    
    create_cluster
    wait_for_cluster
    install_cert_manager
    install_crds
    build_and_deploy_operator
    print_next_steps
}

main "$@"
