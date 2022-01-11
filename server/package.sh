#!/bin/sh

rm ~/Downloads/cloudfunction-$1.zip

case $1 in
	add)
        zip -j ~/Downloads/cloudfunction-$1.zip go.mod ./p/add.go ./p/gist.go ./p/add_utils.go ./p/query_utils.go ./p/utils.go ./p/gist.go
        break
		;;
	client_error)
		zip -j ~/Downloads/cloudfunction-$1.zip go.mod ./p/client_error.go ./p/utils.go
		break
		;;
    query)
        zip -j ~/Downloads/cloudfunction-$1.zip go.mod ./p/query.go ./p/query_utils.go ./p/utils.go
        break
        ;;    
    report)
        zip -j ~/Downloads/cloudfunction-$1.zip go.mod ./p/report.go ./p/add_utils.go ./p/utils.go
        break
        ;;    
    usage)
        zip -j ~/Downloads/cloudfunction-$1.zip go.mod ./p/usage.go ./p/utils.go
        break
        ;;    
  esac
