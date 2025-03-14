# K8s-bench Evaluation Results

## Model Performance Summary

| Model | chat-based Success | chat-based Fail | react Success | react Fail |
|-------|------------|-----------|------------|-----------|
| gemini-2.0-flash | 7 | 3 | 8 | 2 |
| gemini-2.0-flash-thinking-exp-01-21 | 0 | 0 | 7 | 3 |
| gemma-3-27b-it | 0 | 0 | 8 | 2 |
| **Total** | 7 | 3 | 23 | 7 |

## Overall Summary

- Total: 40
- Success: 30 (75%)
- Fail: 10 (25%)

## Strategy: chat-based

| Task | Provider | Model | Result | Error |
|------|----------|-------|--------|-------|
| configure-ingress | gemini | gemini-2.0-flash | ✅ success |  |
| create-network-policy | gemini | gemini-2.0-flash | ❌ fail |  |
| create-pod | gemini | gemini-2.0-flash | ✅ success |  |
| create-pod-mount-configmaps | gemini | gemini-2.0-flash | ✅ success |  |
| create-pod-resources-limits | gemini | gemini-2.0-flash | ✅ success |  |
| fix-crashloop | gemini | gemini-2.0-flash | ✅ success |  |
| fix-image-pull | gemini | gemini-2.0-flash | ✅ success |  |
| fix-service-routing | gemini | gemini-2.0-flash | ❌ fail | exit status 1 |
| scale-deployment | gemini | gemini-2.0-flash | ❌ fail |  |
| scale-down-deployment | gemini | gemini-2.0-flash | ✅ success |  |

**chat-based Summary**

- Total: 10
- Success: 7 (70%)
- Fail: 3 (30%)

## Strategy: react

| Task | Provider | Model | Result | Error |
|------|----------|-------|--------|-------|
| configure-ingress | gemini | gemini-2.0-flash | ✅ success |  |
| configure-ingress | gemini | gemini-2.0-flash-thinking-exp-01-21 | ❌ fail |  |
| configure-ingress | gemini | gemma-3-27b-it | ✅ success |  |
| create-network-policy | gemini | gemini-2.0-flash | ❌ fail |  |
| create-network-policy | gemini | gemini-2.0-flash-thinking-exp-01-21 | ❌ fail |  |
| create-network-policy | gemini | gemma-3-27b-it | ❌ fail |  |
| create-pod | gemini | gemini-2.0-flash | ✅ success |  |
| create-pod | gemini | gemini-2.0-flash-thinking-exp-01-21 | ✅ success |  |
| create-pod | gemini | gemma-3-27b-it | ✅ success |  |
| create-pod-mount-configmaps | gemini | gemini-2.0-flash | ✅ success |  |
| create-pod-mount-configmaps | gemini | gemini-2.0-flash-thinking-exp-01-21 | ✅ success |  |
| create-pod-mount-configmaps | gemini | gemma-3-27b-it | ✅ success |  |
| create-pod-resources-limits | gemini | gemini-2.0-flash | ✅ success |  |
| create-pod-resources-limits | gemini | gemini-2.0-flash-thinking-exp-01-21 | ✅ success |  |
| create-pod-resources-limits | gemini | gemma-3-27b-it | ✅ success |  |
| fix-crashloop | gemini | gemini-2.0-flash | ✅ success |  |
| fix-crashloop | gemini | gemini-2.0-flash-thinking-exp-01-21 | ✅ success |  |
| fix-crashloop | gemini | gemma-3-27b-it | ✅ success |  |
| fix-image-pull | gemini | gemini-2.0-flash | ✅ success |  |
| fix-image-pull | gemini | gemini-2.0-flash-thinking-exp-01-21 | ✅ success |  |
| fix-image-pull | gemini | gemma-3-27b-it | ✅ success |  |
| fix-service-routing | gemini | gemini-2.0-flash | ❌ fail | exit status 1 |
| fix-service-routing | gemini | gemini-2.0-flash-thinking-exp-01-21 | ❌ fail |  |
| fix-service-routing | gemini | gemma-3-27b-it | ❌ fail |  |
| scale-deployment | gemini | gemini-2.0-flash | ✅ success |  |
| scale-deployment | gemini | gemini-2.0-flash-thinking-exp-01-21 | ✅ success |  |
| scale-deployment | gemini | gemma-3-27b-it | ✅ success |  |
| scale-down-deployment | gemini | gemini-2.0-flash | ✅ success |  |
| scale-down-deployment | gemini | gemini-2.0-flash-thinking-exp-01-21 | ✅ success |  |
| scale-down-deployment | gemini | gemma-3-27b-it | ✅ success |  |

**react Summary**

- Total: 30
- Success: 23 (76%)
- Fail: 7 (23%)

---

_Report generated on March 14, 2025 at 11:04 AM_
