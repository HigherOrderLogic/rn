import numpy as np
import requests
from urllib3.util import parse_url

arr = np.arange(10, dtype=np.int64)
print("numpy_sum:", int(arr.sum()))
print("numpy_dot:", int(arr.dot(arr)))

print("has_session:", hasattr(requests, "Session"))
url = parse_url("https://example.com/a/b?q=1")
print("url_host:", url.host)
print("url_path:", url.path)
