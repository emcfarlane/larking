load("std.star", "blob", openapi="net/openapi")

api = openapi.open(addr, spec, client=client)

#print(dir(api))
#bkt = blob.open("file://./testdata")

def test_get(assert):
    rsp = client.get("https://petstore.swagger.io/v2/pet/1")
    print(rsp)


def test_openapi(assert):
    pets = api.FindPetsByStatus(status=["available"])
    pets_by_id = {pet.id: pet for pet in pets}

    pet = pets[0]
    id = pet.id

    print("pet", pets_by_id[id])

    # update pet with generated name
    filename = "pixel.jpg"
    rsp = api.POST_pet_petId_uploadImage(petId=id, file=filename)
    assert.eq(rsp.code, 200)

    # get pet by id
    rsp = api.GetPetById(petId=id)
    assert.eq(rsp.id, id)

    # knight pet with generated name
    pet.name = "Sir " + pet.name
    rsp = api.UpdatePet(body=pet)
    assert.eq(None, rsp)

    # delete pet
    rsp = api.DeletePet(petId=id)
    assert.eq(None, rsp)
