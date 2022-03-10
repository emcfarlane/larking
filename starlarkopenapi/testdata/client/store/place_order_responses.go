// Code generated by go-swagger; DO NOT EDIT.

package store

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/emcfarlane/larking/starlarkopenapi/testdata/models"
)

// PlaceOrderReader is a Reader for the PlaceOrder structure.
type PlaceOrderReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PlaceOrderReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewPlaceOrderOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 400:
		result := NewPlaceOrderBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	default:
		return nil, runtime.NewAPIError("response status code does not match any response statuses defined for this endpoint in the swagger spec", response, response.Code())
	}
}

// NewPlaceOrderOK creates a PlaceOrderOK with default headers values
func NewPlaceOrderOK() *PlaceOrderOK {
	return &PlaceOrderOK{}
}

/* PlaceOrderOK describes a response with status code 200, with default header values.

successful operation
*/
type PlaceOrderOK struct {
	Payload *models.Order
}

func (o *PlaceOrderOK) Error() string {
	return fmt.Sprintf("[POST /store/order][%d] placeOrderOK  %+v", 200, o.Payload)
}
func (o *PlaceOrderOK) GetPayload() *models.Order {
	return o.Payload
}

func (o *PlaceOrderOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Order)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPlaceOrderBadRequest creates a PlaceOrderBadRequest with default headers values
func NewPlaceOrderBadRequest() *PlaceOrderBadRequest {
	return &PlaceOrderBadRequest{}
}

/* PlaceOrderBadRequest describes a response with status code 400, with default header values.

Invalid Order
*/
type PlaceOrderBadRequest struct {
}

func (o *PlaceOrderBadRequest) Error() string {
	return fmt.Sprintf("[POST /store/order][%d] placeOrderBadRequest ", 400)
}

func (o *PlaceOrderBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}