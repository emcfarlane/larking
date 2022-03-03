load("assert.star", "assert")

client = openapi.open(addr, openapi_path)

dir(client)

pets = client.pet.find_pets_by_status(status = ["available"])
print("pets", pets)
