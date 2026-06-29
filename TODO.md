## List of pending items
* when CPI is used then provider id on provisioned node does not match the machine generated provider id which breaks provisioning

* verify provisioning with 3rd party csi

* In HA controlplane provisioning, the 2nd node did not get providerID set on node, as a result cluster provisioning was stuck. Seems like apart from first node, none of the others get the provider id patched

* Tidy up deletion of objects, VM pvc's are not cleaned up

* deletion is stuck if provisioning has failed