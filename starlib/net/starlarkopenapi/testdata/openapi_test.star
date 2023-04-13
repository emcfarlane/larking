load("std.star", "blob", openapi="net/openapi")

api = openapi.open(addr, spec, client=client)


def test_openapi(assert):
    pets = api.FindPetsByStatus(status="available")
    pets_by_id = {pet.id: pet for pet in pets}

    pet = pets[0]
    id = pet.id

    print("pet", pets_by_id[id])

    # get pet by id
    rsp = api.GetPetById(petId=id)
    assert.eq(rsp.id, id)

    # knight pet with generated name
    pet.name = "Sir " + pet.name
    rsp = api.UpdatePet(body=pet)
    assert.eq(pet, rsp)

    # delete pet
    rsp = api.DeletePet(petId=id)
    assert.eq(None, rsp)

    print("done")
