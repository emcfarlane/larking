#load("blob.star", "blob")

api = openapi.open(spec_var, client = client)

print(dir(api))

bkt = blob.open("file://./testdata")

def test_openapi(t):
    def find_available(t):
        pets = api.pet.find_pets_by_status(status = ["available"])
        pets_by_id = {}
        for pet in pets:
            pets_by_id[pet.id] = pet
        return pets_by_id

    pets_available = t.run("find_pets", find_available)
    ids = pets_available.keys()
    first_id = ids[0]
    print("pet", pets_available[first_id])

    def post_image(t):
        filename = "pixel.jpg"
        img = bkt.read_all(filename)
        print("file", (filename, img))
        rsp = api.pet.post_pet_pet_id_upload_image(petId = first_id, file = (filename, img))
        print(rsp)

    t.run("post_image", post_image)

    def get_pet(t):
        rsp = api.pet.get_pet_by_id(petId = first_id)
        print(rsp)

    t.run("get_pet", get_pet)

    pet = pets_available[first_id]

    def update_pet(t):
        pet.name = "Sir " + pet.name
        rsp = api.pet.update_pet(body = pet)
        print(rsp)

    t.run("update_pet", update_pet)

    def delete_pet(t):
        rsp = api.pet.delete_pet(petId = first_id)
        print(rsp)

    t.run("delete_pet", delete_pet)
