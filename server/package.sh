#!/bin/sh

case $1 in
	add)
		echo "add"
		;;
	client_errors)
		echo "client_errors"
		break
		;;
	postprocess)
		echo "postprocess"
		;;
    query)
        echo "query"
        ;;    
    report)
        echo "report"
        ;;    
    usage)
        echo "usage"
        ;;    
  esac
