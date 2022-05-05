load("http.star", "http")
load("openapi.star", "openapi")

spec_var = "https://petstore.swagger.io/v2/swagger.json"

# clients can be customized to alter the request or response.
# This takes a request and adds basic_auth, it also logs the requests.
def client_do(req):
    print(req)  # <request GET https://petstore.swagger.io/v2/pet/findByStatus?status=available>
    req.basic_auth = ("username", "password")  # encodes basic auth
    print(req.header)  # map[Accepts:[application/json] Authorization:[Basic dXNlcm5hbWU6cGFzc3dvcmQ=] Content-Type:[]]

    # use the default client as the transport.
    rsp = http.default_client.do(req)
    print(rsp)  # <response 200 OK>
    return rsp

client = http.client(client_do)

api = openapi.open(spec_var, client = client)

print("api", api)  # api <client "https://petstore.swagger.io/v2/swagger.json">

for svc in dir(api):
    print(svc, [m for m in dir(getattr(api, svc))])

# pet ["add_pet", "delete_pet", "find_pets_by_status", "find_pets_by_tags", "get_pet_by_id", "update_pet", "update_pet_with_form", "upload_file"]
# store ["delete_order", "get_inventory", "get_order_by_id", "place_order"]
# user ["create_user", "create_users_with_array_input", "create_users_with_list_input", "delete_user", "get_user_by_name", "login_user", "logout_user", "update_user"]

pets = api.pet.find_pets_by_status(status = ["available"])
print("%d pets available" % len(pets))  # 573 pets available
