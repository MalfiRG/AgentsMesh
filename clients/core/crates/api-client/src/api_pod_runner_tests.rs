#[cfg(test)]
mod api_pod_runner_tests {
    // Pod + runner REST mocks removed after R5-7 (list_runner_pods now
    // forwards to ListPods Connect) and R5-8 (get_runner_auth_status +
    // authorize_runner moved to proto.runner_api.v1). Connect handler
    // coverage lives under backend/internal/api/connect/{pod,runner}.
    //
    // What stays client-side: connect_call's translation of Connect error
    // bodies into ApiError::Http, which front-ends rely on for error codes
    // (e.g. resource_exhausted → quota messaging in the create-pod dialog).

    use std::sync::Arc;

    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    use agentsmesh_types::proto_pod_v1::CreatePodRequest;

    use crate::{ApiClient, ApiError, AuthTokenStore};

    struct StaticTokenStore;
    impl AuthTokenStore for StaticTokenStore {
        fn get_token(&self) -> Option<String> {
            Some("tok".into())
        }
        fn get_refresh_token(&self) -> Option<String> {
            None
        }
        fn set_tokens(&self, _t: String, _r: String, _e: Option<i64>) {}
        fn clear_tokens(&self) {}
        fn get_current_org_slug(&self) -> Option<String> {
            None
        }
    }

    fn client(server: &MockServer) -> ApiClient {
        ApiClient::new(server.uri(), Arc::new(StaticTokenStore))
    }

    #[tokio::test]
    async fn create_pod_connect_parses_connect_error_body() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/proto.pod.v1.PodService/CreatePod"))
            .respond_with(ResponseTemplate::new(429).set_body_json(serde_json::json!({
                "code": "resource_exhausted",
                "message": "quota exceeded: concurrent pod limit reached (5 of 5 in use)"
            })))
            .mount(&server)
            .await;

        let err = client(&server)
            .create_pod_connect(&CreatePodRequest::default())
            .await
            .unwrap_err();

        match err {
            ApiError::Http {
                status,
                code,
                server_message,
                ..
            } => {
                assert_eq!(status, 429);
                assert_eq!(code.as_deref(), Some("resource_exhausted"));
                assert_eq!(
                    server_message.as_deref(),
                    Some("quota exceeded: concurrent pod limit reached (5 of 5 in use)")
                );
            }
            other => panic!("expected ApiError::Http, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn create_pod_connect_keeps_raw_body_when_not_connect_json() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/proto.pod.v1.PodService/CreatePod"))
            .respond_with(ResponseTemplate::new(502).set_body_string("bad gateway"))
            .mount(&server)
            .await;

        let err = client(&server)
            .create_pod_connect(&CreatePodRequest::default())
            .await
            .unwrap_err();

        match err {
            ApiError::Http {
                status,
                code,
                server_message,
                ..
            } => {
                assert_eq!(status, 502);
                assert_eq!(code, None);
                assert_eq!(server_message.as_deref(), Some("bad gateway"));
            }
            other => panic!("expected ApiError::Http, got {other:?}"),
        }
    }
}
