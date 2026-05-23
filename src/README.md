# Rikami controller

Functionality:

- secured with HMAC token per user (for now from env, later from db)
- check local vessels already in image
- if not present look for vessel in repo and download
- generate vessel with specified name and version (thru rika cli)
- auto push thru rika cli (pull -> add -> commit -> push)

Requests format:

Authorization: Bearer userID | base64 -d

```json
{
  "vessel": "wp"
  "version": 0.1.13
  "name": "wp1"
  "envs": [{
    "envName": ".env.some.secret"
    "envVals": {
      "somekey1": "someval1"
    }
  }]
}
```


