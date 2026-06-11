// Facade re-export of the binding Connect-RPC adapter. Business code imports
// from here (or from the `@/lib/api` barrel) so the wire-shape layer stays
// internal to the facade boundary. Tests mock this path.

export {
  requestBindingConnect,
  acceptBindingConnect,
  rejectBindingConnect,
  unbindConnect,
  requestScopesConnect,
  approveScopesConnect,
  listBindingsConnect,
  getPendingBindingsConnect,
  getBoundPodsConnect,
  checkBindingConnect,
  type PodBinding,
} from "../connect/bindingConnect";
